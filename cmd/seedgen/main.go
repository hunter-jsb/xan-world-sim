// seedgen replaces region_cells and river_cells in world.db with a
// procedurally-generated world from a seed and an era.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/world"
)

func main() {
	seed := flag.Int64("seed", 0, "RNG seed (0 = pick a random one and print it)")
	dbPath := flag.String("db", "world.db", "path to world.db")
	eraFlag := flag.String("era", "now", `world era: "now" or "205kya"`)
	flag.Parse()

	era, err := world.ParseEra(*eraFlag)
	if err != nil {
		log.Fatalf("%v", err)
	}

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

	w := world.Generate(*seed, era)
	if err := world.Persist(context.Background(), conn, w); err != nil {
		log.Fatalf("persist: %v", err)
	}

	fmt.Printf("seed=%d era=%s  regions=%d rivers=%d  written to %s\n",
		*seed, w.Era, len(w.Regions), len(w.Rivers), *dbPath)
}
