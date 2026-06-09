package main

import (
	"container/heap"
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

	// Raw world data — updated on each regen, used for instant
	// cursor re-renders without re-querying the DB.
	cells    []db.GetCellsInBoundsRow
	rivers   []db.GetRiverCellsInBoundsRow
	roads    []db.GetRoadCellsInBoundsRow
	seats    []db.GetSeatsInBoundsRow
	features []db.GetNamedFeaturesInBoundsRow

	// Lookup maps built from raw data for O(1) cursor inspection.
	cellAt    map[[2]int64]db.GetCellsInBoundsRow
	riverAt   map[[2]int64]string // coord → river name
	seatAt    map[[2]int64]db.GetSeatsInBoundsRow
	featureAt map[[2]int64]string // coord → feature name (kind in cellAt)

	gridBuf *render.GridBuf // pre-rendered grid; Render() is fast on cursor moves
	mapStr  string
	legend  string
	seed   int64
	kya    int
	era    world.Era
	status string

	// Cursor position on the map.
	curX, curY int64

	// Expedition pathfinding: expStart is the journey origin (nil = inactive).
	// expPath is the live Dijkstra route from start to cursor, recomputed on
	// every cursor move. dangerMap is pre-built per regen from lair features.
	expStart  *[2]int64
	expPath   []render.PathCell
	dangerMap map[[2]int64]int

	minX, minY, maxX, maxY int64
}

// regenMsg carries the raw query results from a regen Cmd. Rendering
// is deferred to the Update handler so cursor movements can re-render
// from stored data without re-querying the DB.
type regenMsg struct {
	cells    []db.GetCellsInBoundsRow
	rivers   []db.GetRiverCellsInBoundsRow
	roads    []db.GetRoadCellsInBoundsRow
	seats    []db.GetSeatsInBoundsRow
	features []db.GetNamedFeaturesInBoundsRow
	seed     int64
	kya      int
	era      world.Era
	err      error
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			newSeed := time.Now().UnixNano()
			m.seed = newSeed
			m.status = "rerolling..."
			return m, m.regen(newSeed, m.kya)
		case "e":
			next := world.KyaNow
			if m.kya == world.KyaNow {
				next = world.KyaOldWorld
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("jumping to %dkya...", next)
			return m, m.regen(m.seed, next)

		// Time-scrubbing: kya = kiloyears *before* present.
		// `]` / right = forward in time (toward present, kya decreases).
		// `[` / left  = backward in time (toward LGM, kya increases).
		case "]", "right":
			next := clampKya(m.kya - stepSmall)
			if next == m.kya {
				m.status = "at 0kya (present)"
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("→ %dkya", next)
			return m, m.regen(m.seed, next)
		case "[", "left":
			next := clampKya(m.kya + stepSmall)
			if next == m.kya {
				m.status = fmt.Sprintf("at %dkya (past cap)", world.KyaMax)
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("← %dkya", next)
			return m, m.regen(m.seed, next)
		case "}", "shift+right":
			next := clampKya(m.kya - stepBig)
			if next == m.kya {
				m.status = "at 0kya (present)"
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("→→ %dkya", next)
			return m, m.regen(m.seed, next)
		case "{", "shift+left":
			next := clampKya(m.kya + stepBig)
			if next == m.kya {
				m.status = fmt.Sprintf("at %dkya (past cap)", world.KyaMax)
				return m, nil
			}
			m.kya, m.era = next, world.EraForKya(next)
			m.status = fmt.Sprintf("←← %dkya", next)
			return m, m.regen(m.seed, next)

		// Expedition: s sets/clears the journey start at the cursor.
		// While active, every cursor move recomputes the Dijkstra path.
		case "s":
			if m.expStart == nil {
				pos := [2]int64{m.curX, m.curY}
				m.expStart = &pos
				m.expPath = []render.PathCell{{X: m.curX, Y: m.curY, G: '@'}}
				m.status = fmt.Sprintf("expedition start set at (%d,%d)", m.curX, m.curY)
			} else {
				m.expStart = nil
				m.expPath = nil
				m.status = "expedition cleared"
			}
			m.mapStr = m.buildMap()

		// Cursor navigation — hjkl, instant (no regen needed).
		case "h":
			if m.curX > m.minX {
				m.curX--
				m.recomputeExpPath()
				m.mapStr = m.buildMap()
			}
		case "l":
			if m.curX < m.maxX {
				m.curX++
				m.recomputeExpPath()
				m.mapStr = m.buildMap()
			}
		case "k":
			if m.curY > m.minY {
				m.curY--
				m.recomputeExpPath()
				m.mapStr = m.buildMap()
			}
		case "j":
			if m.curY < m.maxY {
				m.curY++
				m.recomputeExpPath()
				m.mapStr = m.buildMap()
			}
		}

	case regenMsg:
		if msg.err != nil {
			m.status = "regen error: " + msg.err.Error()
			return m, nil
		}
		// Discard stale renders — user may have pressed more keys while
		// this Cmd was running, advancing m.kya/m.seed past this target.
		if msg.kya != m.kya || msg.seed != m.seed {
			return m, nil
		}
		m.cells = msg.cells
		m.rivers = msg.rivers
		m.roads = msg.roads
		m.seats = msg.seats
		m.features = msg.features
		m.buildLookups()
		m.gridBuf = render.BuildGridBuf(m.cells, m.rivers, m.roads, m.minX, m.minY, m.maxX, m.maxY)
		// World changed — terrain costs shift, so stale paths are misleading.
		m.expStart = nil
		m.expPath = nil
		m.mapStr = m.buildMap()
		m.era = msg.era
		m.status = ""
		return m, nil
	}
	return m, nil
}

// buildLookups rebuilds the O(1) coord→data maps from the stored slices.
func (m *model) buildLookups() {
	m.cellAt = make(map[[2]int64]db.GetCellsInBoundsRow, len(m.cells))
	for _, c := range m.cells {
		m.cellAt[[2]int64{c.X, c.Y}] = c
	}
	m.riverAt = make(map[[2]int64]string, len(m.rivers))
	for _, r := range m.rivers {
		m.riverAt[[2]int64{r.X, r.Y}] = r.RiverName
	}
	m.seatAt = make(map[[2]int64]db.GetSeatsInBoundsRow, len(m.seats))
	for _, s := range m.seats {
		m.seatAt[[2]int64{s.X, s.Y}] = s
	}
	m.featureAt = make(map[[2]int64]string, len(m.features))
	for _, f := range m.features {
		m.featureAt[[2]int64{f.X, f.Y}] = f.Name
	}
	m.dangerMap = buildDangerMap(m.features)
}

// buildMap renders the grid using the cached GridBuf — only the cursor
// row is re-rendered, so cursor moves are ~50× faster than a full regen.
func (m *model) buildMap() string {
	if m.gridBuf == nil {
		return ""
	}
	return m.gridBuf.Render(m.curX, m.curY, m.expPath)
}

// cellInfoAt assembles a CellInfo for the cursor position.
func (m *model) cellInfoAt(x, y int64) render.CellInfo {
	info := render.CellInfo{X: x, Y: y}
	if c, ok := m.cellAt[[2]int64{x, y}]; ok {
		info.Kind = c.Kind
		info.Elev = c.Elevation
	}
	if rn, ok := m.riverAt[[2]int64{x, y}]; ok {
		info.RiverName = rn
	}
	if s, ok := m.seatAt[[2]int64{x, y}]; ok {
		info.SeatName = s.Name
		info.SeatPressure = s.Pressure
	}
	if fn, ok := m.featureAt[[2]int64{x, y}]; ok {
		info.FeatureName = fn
	}
	return info
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
			return regenMsg{err: fmt.Errorf("cells: %w", err)}
		}
		rivers, err := m.q.GetRiverCellsInBounds(m.ctx, db.GetRiverCellsInBoundsParams{
			X: m.minX, X_2: m.maxX, Y: m.minY, Y_2: m.maxY,
		})
		if err != nil {
			return regenMsg{err: fmt.Errorf("rivers: %w", err)}
		}
		roads, err := m.q.GetRoadCellsInBounds(m.ctx, m.minX, m.maxX, m.minY, m.maxY)
		if err != nil {
			return regenMsg{err: fmt.Errorf("roads: %w", err)}
		}
		seats, err := m.q.GetSeatsInBounds(m.ctx, m.minX, m.maxX, m.minY, m.maxY)
		if err != nil {
			return regenMsg{err: fmt.Errorf("seats: %w", err)}
		}
		features, err := m.q.GetNamedFeaturesInBounds(m.ctx, m.minX, m.maxX, m.minY, m.maxY)
		if err != nil {
			return regenMsg{err: fmt.Errorf("features: %w", err)}
		}
		simLog("ok seed=%d kya=%d cells=%d rivers=%d roads=%d seats=%d features=%d",
			seed, kya, len(cells), len(rivers), len(roads), len(seats), len(features))
		return regenMsg{
			cells: cells, rivers: rivers, roads: roads,
			seats: seats, features: features,
			seed: seed, kya: kya, era: w.Era,
		}
	}
}

// simLog appends a timestamped line to /tmp/xan-sim.log.
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
	b.WriteString(render.InfoPanel(m.cellInfoAt(m.curX, m.curY)))
	b.WriteString("\n\n")
	b.WriteString(m.legend)
	b.WriteString("\n\n")
	expHint := "s expedition"
	if m.expStart != nil {
		expHint = fmt.Sprintf("s clear expedition (%d steps)", max(0, len(m.expPath)-1))
	}
	b.WriteString(dimStyle.Render("hjkl cursor   "+expHint+"   ] / [ ±5ka   } / { ±25ka   r reroll   e now/LGM   q quit"))
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
	seats, err := q.GetSeatsInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		log.Fatalf("get seats: %v", err)
	}
	features, err := q.GetNamedFeaturesInBounds(ctx, minX, maxX, minY, maxY)
	if err != nil {
		log.Fatalf("get features: %v", err)
	}

	era := world.EraForKya(kya)

	if *printOnce {
		fmt.Println(render.Title("xan-world-sim — the cradle"))
		fmt.Println()
		fmt.Println(render.Grid(cells, rivers, roads, minX, minY, maxX, maxY, -1, -1))
		fmt.Println()
		fmt.Println(render.Legend())
		fmt.Printf("\nt: %dkya (%s)   glacial: %.2f   seed: %d\n",
			kya, era, world.GlacialIndex(kya), seed)
		return
	}

	// Cursor starts at map center.
	initCurX := (minX + maxX) / 2
	initCurY := (minY + maxY) / 2

	m := model{
		ctx: ctx, conn: conn, q: q,
		regenMu:  &sync.Mutex{},
		cells:    cells,
		rivers:   rivers,
		roads:    roads,
		seats:    seats,
		features: features,
		legend:   render.Legend(),
		seed:     seed,
		kya:      kya,
		era:      era,
		curX:     initCurX,
		curY:     initCurY,
		minX:     minX,
		minY:     minY,
		maxX:     maxX,
		maxY:     maxY,
	}
	m.buildLookups()
	m.gridBuf = render.BuildGridBuf(cells, rivers, roads, minX, minY, maxX, maxY)
	m.mapStr = m.buildMap()

	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("tea: %v", err)
	}
}

// expHeapItem is a Dijkstra priority queue entry.
type expHeapItem struct {
	x, y int64
	cost int
}
type expHeap []expHeapItem

func (h expHeap) Len() int            { return len(h) }
func (h expHeap) Less(i, j int) bool  { return h[i].cost < h[j].cost }
func (h expHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *expHeap) Push(x interface{}) { *h = append(*h, x.(expHeapItem)) }
func (h *expHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// recomputeExpPath re-runs Dijkstra from expStart to the current cursor.
// No-ops when no expedition is active.
func (m *model) recomputeExpPath() {
	if m.expStart == nil {
		return
	}
	sx, sy := m.expStart[0], m.expStart[1]
	if sx == m.curX && sy == m.curY {
		m.expPath = []render.PathCell{{X: sx, Y: sy, G: '@'}}
		return
	}
	path := m.computePath(sx, sy, m.curX, m.curY)
	if path == nil {
		// No route found — keep start marker only.
		m.expPath = []render.PathCell{{X: sx, Y: sy, G: '@'}}
		m.status = "no route"
	} else {
		m.expPath = path
		m.status = fmt.Sprintf("expedition: %d steps", len(path)-1)
	}
}

// buildDangerMap pre-computes per-cell dragon danger scores from all lair
// features. Dens radiate danger within radius 12 (scale ×3), nests 8 (×2),
// rookeries 6 (×1). Overlapping zones take the maximum score.
func buildDangerMap(features []db.GetNamedFeaturesInBoundsRow) map[[2]int64]int {
	danger := make(map[[2]int64]int)
	for _, f := range features {
		var radius, scale int
		switch f.Kind {
		case "den":
			radius, scale = 12, 3
		case "nest":
			radius, scale = 8, 2
		case "rookery":
			radius, scale = 6, 1
		default:
			continue
		}
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				cheb := int64(dx)
				if cheb < 0 {
					cheb = -cheb
				}
				if ady := int64(dy); ady < 0 {
					if -ady > cheb {
						cheb = -ady
					}
				} else if ady > cheb {
					cheb = ady
				}
				d := (radius - int(cheb)) * scale
				if d <= 0 {
					continue
				}
				k := [2]int64{f.X + int64(dx), f.Y + int64(dy)}
				if d > danger[k] {
					danger[k] = d
				}
			}
		}
	}
	return danger
}

// pathCellCost returns the movement cost to enter (x, y), or -1 if impassable.
func (m *model) pathCellCost(x, y int64) int {
	coord := [2]int64{x, y}
	danger := m.dangerMap[coord]

	// Rivers make any passable terrain trivial to traverse.
	if m.riverAt[coord] != "" {
		return 1 + danger
	}

	c, ok := m.cellAt[coord]
	if !ok {
		return -1
	}
	var base int
	switch c.Kind {
	case "seat", "march", "headwater", "outhold", "reach":
		base = 2
	case "pass":
		base = 3
	case "cradle", "forest", "tundra", "agraria", "agraria_upland":
		base = 4
	case "foothill":
		base = 5
	case "doab":
		base = 6
	case "marsh":
		base = 8
	case "plateau":
		base = 15
	case "den", "nest", "rookery":
		base = 25
	default:
		// mountain, cliff, sea_brine, sea_eastern, glacier, lake, drowned
		return -1
	}
	return base + danger
}

// computePath runs weighted Dijkstra from (sx,sy) to (ex,ey) and returns
// the path as PathCells with directional glyphs. Returns nil if unreachable.
func (m *model) computePath(sx, sy, ex, ey int64) []render.PathCell {
	type coord = [2]int64
	start := coord{sx, sy}
	end := coord{ex, ey}

	dist := make(map[coord]int, 512)
	prev := make(map[coord]coord, 512)
	dist[start] = 0

	h := &expHeap{{sx, sy, 0}}
	heap.Init(h)

	dirs := [8][2]int64{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}}

	for h.Len() > 0 {
		cur := heap.Pop(h).(expHeapItem)
		cc := coord{cur.x, cur.y}
		if cc == end {
			break
		}
		if cur.cost > dist[cc] {
			continue // stale entry
		}
		for _, d := range dirs {
			nx, ny := cur.x+d[0], cur.y+d[1]
			if nx < m.minX || nx > m.maxX || ny < m.minY || ny > m.maxY {
				continue
			}
			nc := coord{nx, ny}
			cost := m.pathCellCost(nx, ny)
			if cost < 0 {
				continue
			}
			newDist := dist[cc] + cost
			if d, seen := dist[nc]; !seen || newDist < d {
				dist[nc] = newDist
				prev[nc] = cc
				heap.Push(h, expHeapItem{nx, ny, newDist})
			}
		}
	}

	if _, reached := dist[end]; !reached {
		return nil
	}

	var steps []coord
	for c := end; c != start; c = prev[c] {
		steps = append(steps, c)
	}
	steps = append(steps, start)
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}

	result := make([]render.PathCell, len(steps))
	for i, c := range steps {
		var g rune
		switch {
		case i == 0:
			g = '@'
		case i == len(steps)-1:
			g = 'X'
		default:
			next := steps[i+1]
			g = render.DirectionalGlyph(int(next[0]-c[0]), int(next[1]-c[1]))
		}
		result[i] = render.PathCell{X: c[0], Y: c[1], G: g}
	}
	return result
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
