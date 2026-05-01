// seedgen replaces region_cells and river_cells in world.db with a
// procedurally-generated world from a seed. Run after `goose up`.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/db"
	"github.com/hunterjsb/xan-world-sim/internal/world"
)

func main() {
	seed := flag.Int64("seed", 0, "RNG seed (0 = pick a random one and print it)")
	dbPath := flag.String("db", "world.db", "path to world.db")
	flag.Parse()

	if *seed == 0 {
		*seed = time.Now().UnixNano()
		fmt.Fprintf(os.Stderr, "no --seed given; using %d\n", *seed)
	}

	conn, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		log.Fatalf("ping db: %v", err)
	}
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatalf("enable fks: %v", err)
	}

	w := world.Generate(*seed)

	ctx := context.Background()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		log.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM river_cells"); err != nil {
		log.Fatalf("clear rivers: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM region_cells"); err != nil {
		log.Fatalf("clear regions: %v", err)
	}

	q := db.New(tx)
	for _, c := range w.Regions {
		if err := q.InsertCell(ctx, db.InsertCellParams{
			RegionID: c.RegionID, X: c.X, Y: c.Y,
		}); err != nil {
			log.Fatalf("insert region cell: %v", err)
		}
	}

	rcStmt, err := tx.PrepareContext(ctx,
		`INSERT INTO river_cells (river_id, x, y, ord) VALUES (?, ?, ?, ?)`)
	if err != nil {
		log.Fatalf("prepare river insert: %v", err)
	}
	defer rcStmt.Close()
	for _, r := range w.Rivers {
		if _, err := rcStmt.ExecContext(ctx, r.RiverID, r.X, r.Y, r.Ord); err != nil {
			log.Fatalf("insert river cell: %v", err)
		}
	}

	metaStmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO world_meta (key, value) VALUES (?, ?)`)
	if err != nil {
		log.Fatalf("prepare meta: %v", err)
	}
	defer metaStmt.Close()
	if _, err := metaStmt.ExecContext(ctx, "seed", fmt.Sprint(*seed)); err != nil {
		log.Fatalf("set seed: %v", err)
	}
	if _, err := metaStmt.ExecContext(ctx, "generated_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		log.Fatalf("set generated_at: %v", err)
	}

	if err := tx.Commit(); err != nil {
		log.Fatalf("commit: %v", err)
	}

	fmt.Printf("seed=%d  regions=%d rivers=%d  written to %s\n",
		*seed, len(w.Regions), len(w.Rivers), *dbPath)
	_ = rand.New
}
