package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/render"
)

type model struct {
	mapStr  string
	legend  string
	regions []db.Region
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(render.Title("xan-world-sim — the cradle"))
	b.WriteString("\n\n")
	b.WriteString(m.mapStr)
	b.WriteString("\n\n")
	b.WriteString(m.legend)
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("q/esc to quit"))
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

	if *printOnce {
		fmt.Println(render.Title("xan-world-sim — the cradle"))
		fmt.Println()
		fmt.Println(mapStr)
		fmt.Println()
		fmt.Println(render.Legend())
		return
	}

	m := model{mapStr: mapStr, legend: render.Legend(), regions: regions}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		log.Fatalf("tea: %v", err)
	}
}
