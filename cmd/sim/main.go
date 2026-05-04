package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/migrations"
	"github.com/hunterjsb/xan-world-sim/internal/render"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

const (
	stepSmall = 5
	stepBig   = 25
)

func clampKya(k int) int {
	if k < 0 {
		return 0
	}
	if k > world.KyaMax {
		return world.KyaMax
	}
	return k
}

type model struct {
	ctx  context.Context
	conn *sql.DB
	q    *db.Queries

	// regenMu serializes regen Cmds. Bubble Tea fires Cmds in
	// goroutines, so spamming the kya keys can launch multiple
	// concurrent Generate→Persist→Query pipelines that race on
	// SQLite (which only allows one writer at a time and surfaces
	// the contention as "database is locked").
	regenMu *sync.Mutex

	mapStr string
	legend string
	seed   int64
	kya    int
	era    world.Era
	status string

	minX, minY, maxX, maxY int64
}

type regenMsg struct {
	mapStr string
	seed   int64
	kya    int
	era    world.Era
	err    error
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			m.status = "rerolling..."
			return m, m.regen(time.Now().UnixNano(), m.kya)
		case "e":
			next := world.KyaNow
			if m.kya == world.KyaNow {
				next = world.KyaOldWorld
			}
			m.status = fmt.Sprintf("jumping to %dkya...", next)
			return m, m.regen(m.seed, next)
		// Time-scrubbing convention: kya = kiloyears *before* present.
		// "Right" / `]` = step forward through geological history =
		// further back into the past (kya increases). "Left" / `[` =
		// rewind toward the present (kya decreases). Both ends of the
		// scrub bar surface a status so dead-end key presses aren't
		// silent.
		case "]", "right":
			next := clampKya(m.kya + stepSmall)
			if next == m.kya {
				m.status = fmt.Sprintf("at %dkya (deep-time cap)", world.KyaMax)
				return m, nil
			}
			m.status = fmt.Sprintf("→ %dkya", next)
			return m, m.regen(m.seed, next)
		case "[", "left":
			next := clampKya(m.kya - stepSmall)
			if next == m.kya {
				m.status = "at 0kya (present)"
				return m, nil
			}
			m.status = fmt.Sprintf("← %dkya", next)
			return m, m.regen(m.seed, next)
		case "}", "shift+right":
			next := clampKya(m.kya + stepBig)
			if next == m.kya {
				m.status = fmt.Sprintf("at %dkya (deep-time cap)", world.KyaMax)
				return m, nil
			}
			m.status = fmt.Sprintf("→→ %dkya", next)
			return m, m.regen(m.seed, next)
		case "{", "shift+left":
			next := clampKya(m.kya - stepBig)
			if next == m.kya {
				m.status = "at 0kya (present)"
				return m, nil
			}
			m.status = fmt.Sprintf("←← %dkya", next)
			return m, m.regen(m.seed, next)
		}
	case regenMsg:
		if msg.err != nil {
			m.status = "regen error: " + msg.err.Error()
		} else {
			m.mapStr = msg.mapStr
			m.seed = msg.seed
			m.kya = msg.kya
			m.era = msg.era
			m.status = ""
		}
		return m, nil
	}
	return m, nil
}

func (m model) regen(seed int64, kya int) tea.Cmd {
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				simLog("panic seed=%d kya=%d: %v\n%s", seed, kya, r, debug.Stack())
				msg = regenMsg{err: fmt.Errorf("panic at kya=%d: %v", kya, r)}
			}
		}()
		m.regenMu.Lock()
		defer m.regenMu.Unlock()
		simLog("regen seed=%d kya=%d", seed, kya)
		w := world.Generate(seed, kya)
		if err := world.Persist(m.ctx, m.conn, w); err != nil {
			simLog("persist failed seed=%d kya=%d: %v", seed, kya, err)
			return regenMsg{err: fmt.Errorf("persist: %w", err)}
		}
		cells, err := m.q.GetCellsInBounds(m.ctx, db.GetCellsInBoundsParams{
			X: m.minX, X_2: m.maxX, Y: m.minY, Y_2: m.maxY,
		})
		if err != nil {
			simLog("cells failed seed=%d kya=%d: %v", seed, kya, err)
			return regenMsg{err: fmt.Errorf("cells: %w", err)}
		}
		rivers, err := m.q.GetRiverCellsInBounds(m.ctx, db.GetRiverCellsInBoundsParams{
			X: m.minX, X_2: m.maxX, Y: m.minY, Y_2: m.maxY,
		})
		if err != nil {
			simLog("rivers failed seed=%d kya=%d: %v", seed, kya, err)
			return regenMsg{err: fmt.Errorf("rivers: %w", err)}
		}
		roads, err := m.q.GetRoadCellsInBounds(m.ctx, m.minX, m.maxX, m.minY, m.maxY)
		if err != nil {
			simLog("roads failed seed=%d kya=%d: %v", seed, kya, err)
			return regenMsg{err: fmt.Errorf("roads: %w", err)}
		}
		simLog("ok seed=%d kya=%d cells=%d rivers=%d roads=%d", seed, kya, len(cells), len(rivers), len(roads))
		return regenMsg{
			mapStr: render.Grid(cells, rivers, roads, m.minX, m.minY, m.maxX, m.maxY),
			seed:   seed,
			kya:    kya,
			era:    w.Era,
		}
	}
}

// simLog appends a timestamped line to /tmp/xan-sim.log. Useful for
// capturing TUI errors that flicker through too fast to read on
// screen, or panics that kill the Bubble Tea program.
func simLog(format string, args ...interface{}) {
	f, err := os.OpenFile("/tmp/xan-sim.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s "+format+"\n", append([]interface{}{time.Now().Format("15:04:05.000")}, args...)...)
}

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	seedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Italic(true)
)

func (m model) View() string {
	var b strings.Builder
	title := render.Title("xan-world-sim — the cradle")
	title += dimStyle.Render("   t: ") + seedStyle.Render(fmt.Sprintf("%dkya", m.kya))
	if string(m.era) != "" && string(m.era) != fmt.Sprintf("%dkya", m.kya) {
		title += dimStyle.Render(" (") + seedStyle.Render(string(m.era)) + dimStyle.Render(")")
	}
	gI := world.GlacialIndex(m.kya)
	title += dimStyle.Render("   glacial: ") + seedStyle.Render(fmt.Sprintf("%.2f", gI))
	if m.seed != 0 {
		title += dimStyle.Render("   seed: ") + seedStyle.Render(fmt.Sprintf("%d", m.seed))
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(m.mapStr)
	b.WriteString("\n\n")
	b.WriteString(m.legend)
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("] / [ step ±5ka · } / { step ±25ka · r reroll · e jump now/LGM · q quit"))
	if m.status != "" {
		b.WriteString("   ")
		b.WriteString(statusStyle.Render(m.status))
	}
	return b.String()
}

func main() {
	printOnce := flag.Bool("print", false, "render map once to stdout and exit (no TUI)")
	flag.Parse()

	dbPath := "world.db"
	if v := os.Getenv("XAN_DB"); v != "" {
		dbPath = v
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := conn.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}

	// Auto-apply schema migrations from the embedded FS. Idempotent —
	// goose tracks which migrations have already run. Means a fresh
	// world.db (or no world.db) is fully bootstrapped just by running
	// `go run ./cmd/sim`; no separate goose CLI step required.
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		log.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(conn, "."); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	q := db.New(conn)
	ctx := context.Background()

	const minX, minY = 0, 0
	maxX, maxY := int64(world.Width-1), int64(world.Height-1)

	// Self-bootstrap: read seed+kya from world_meta (defaults to 0 if
	// fresh after `goose up`), regenerate the world from current code,
	// and write through to the DB. This means `goose up && go run
	// ./cmd/sim` always shows what the current code produces, with no
	// stale-DB problem and no required seedgen step.
	seed := readMetaInt(ctx, conn, "seed")
	kya := int(readMetaInt(ctx, conn, "kya"))
	if err := world.Persist(ctx, conn, world.Generate(seed, kya)); err != nil {
		log.Fatalf("bootstrap world: %v", err)
	}

	cells, err := q.GetCellsInBounds(ctx, db.GetCellsInBoundsParams{
		X: minX, X_2: maxX, Y: minY, Y_2: maxY,
	})
	if err != nil {
		log.Fatalf("get cells: %v", err)
	}
	rivers, err := q.GetRiverCellsInBounds(ctx, db.GetRiverCellsInBoundsParams{
		X: minX, X_2: maxX, Y: minY, Y_2: maxY,
	})
	if err != nil {
		log.Fatalf("get river cells: %v", err)
	}
	roads, err := q.GetRoadCellsInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		log.Fatalf("get road cells: %v", err)
	}

	mapStr := render.Grid(cells, rivers, roads, minX, minY, maxX, maxY)

	era := world.EraForKya(kya)

	if *printOnce {
		fmt.Println(render.Title("xan-world-sim — the cradle"))
		fmt.Println()
		fmt.Println(mapStr)
		fmt.Println()
		fmt.Println(render.Legend())
		fmt.Printf("\nt: %dkya (%s)   glacial: %.2f   seed: %d\n",
			kya, era, world.GlacialIndex(kya), seed)
		return
	}

	m := model{
		ctx: ctx, conn: conn, q: q,
		regenMu: &sync.Mutex{},
		mapStr:  mapStr, legend: render.Legend(),
		seed: seed, kya: kya, era: era,
		minX: minX, minY: minY, maxX: maxX, maxY: maxY,
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("tea: %v", err)
	}
}

func readMetaString(ctx context.Context, conn *sql.DB, key string) string {
	var s string
	err := conn.QueryRowContext(ctx, "SELECT value FROM world_meta WHERE key = ?", key).Scan(&s)
	if err != nil {
		return ""
	}
	return s
}

func readMetaInt(ctx context.Context, conn *sql.DB, key string) int64 {
	s := readMetaString(ctx, conn, key)
	if s == "" {
		return 0
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}
