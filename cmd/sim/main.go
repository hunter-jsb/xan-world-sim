package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
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
	var b []byte
	b = append(b, "xan-world-sim — the cradle (placeholder seed)\n\n"...)
	b = append(b, m.mapStr...)
	b = append(b, "\n\n"...)
	b = append(b, m.legend...)
	b = append(b, "\n\nq/esc to quit"...)
	return string(b)
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

	cells, err := q.GetCellsInBounds(ctx, db.GetCellsInBoundsParams{
		X: 0, X_2: 20, Y: 0, Y_2: 20,
	})
	if err != nil {
		log.Fatalf("get cells: %v", err)
	}

	mapStr := render.Grid(cells, 0, 0, 10, 8)

	if *printOnce {
		fmt.Println("xan-world-sim — the cradle (placeholder seed)")
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
