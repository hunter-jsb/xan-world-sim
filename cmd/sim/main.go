package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/render"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

type model struct {
	ctx    context.Context
	conn   *sql.DB
	q      *db.Queries

	mapStr string
	legend string
	seed   int64
	status string

	minX, minY, maxX, maxY int64
}

type rerolledMsg struct {
	mapStr string
	seed   int64
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
			return m, m.reroll()
		}
	case rerolledMsg:
		if msg.err != nil {
			m.status = "reroll error: " + msg.err.Error()
		} else {
			m.mapStr = msg.mapStr
			m.seed = msg.seed
			m.status = ""
		}
		return m, nil
	}
	return m, nil
}

func (m model) reroll() tea.Cmd {
	return func() tea.Msg {
		seed := time.Now().UnixNano()
		w := world.Generate(seed)
		if err := world.Persist(m.ctx, m.conn, w); err != nil {
			return rerolledMsg{err: err}
		}
		cells, err := m.q.GetCellsInBounds(m.ctx, db.GetCellsInBoundsParams{
			X: m.minX, X_2: m.maxX, Y: m.minY, Y_2: m.maxY,
		})
		if err != nil {
			return rerolledMsg{err: err}
		}
		rivers, err := m.q.GetRiverCellsInBounds(m.ctx, db.GetRiverCellsInBoundsParams{
			X: m.minX, X_2: m.maxX, Y: m.minY, Y_2: m.maxY,
		})
		if err != nil {
			return rerolledMsg{err: err}
		}
		return rerolledMsg{
			mapStr: render.Grid(cells, rivers, m.minX, m.minY, m.maxX, m.maxY),
			seed:   seed,
		}
	}
}

var (
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	seedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Italic(true)
)

func (m model) View() string {
	var b strings.Builder
	title := render.Title("xan-world-sim — the cradle")
	if m.seed != 0 {
		title += dimStyle.Render("   seed: ") + seedStyle.Render(fmt.Sprintf("%d", m.seed))
	}
	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(m.mapStr)
	b.WriteString("\n\n")
	b.WriteString(m.legend)
	b.WriteString("\n\n")
	b.WriteString(dimStyle.Render("r reroll · q/esc quit"))
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

	q := db.New(conn)
	ctx := context.Background()

	regions, err := q.ListRegions(ctx)
	if err != nil {
		log.Fatalf("list regions: %v", err)
	}
	if len(regions) == 0 {
		fmt.Fprintln(os.Stderr, "no regions in db — did you run migrations? `goose -dir migrations sqlite3 world.db up`")
		os.Exit(1)
	}

	const minX, minY, maxX, maxY = 0, 0, 59, 21

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

	mapStr := render.Grid(cells, rivers, minX, minY, maxX, maxY)

	seed := readSeed(ctx, conn)

	if *printOnce {
		fmt.Println(render.Title("xan-world-sim — the cradle"))
		fmt.Println()
		fmt.Println(mapStr)
		fmt.Println()
		fmt.Println(render.Legend())
		if seed != 0 {
			fmt.Printf("\nseed: %d\n", seed)
		}
		return
	}

	m := model{
		ctx: ctx, conn: conn, q: q,
		mapStr: mapStr, legend: render.Legend(), seed: seed,
		minX: minX, minY: minY, maxX: maxX, maxY: maxY,
	}
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("tea: %v", err)
	}
}

func readSeed(ctx context.Context, conn *sql.DB) int64 {
	var s string
	err := conn.QueryRowContext(ctx, "SELECT value FROM world_meta WHERE key = 'seed'").Scan(&s)
	if err != nil {
		return 0
	}
	var seed int64
	fmt.Sscanf(s, "%d", &seed)
	return seed
}
