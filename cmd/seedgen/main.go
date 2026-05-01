// seedgen replaces region_cells and river_cells in world.db with a
// procedurally-generated world from a seed and a moment in time.
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
	kya := flag.Int("kya", 0, "kiloyears before present (0 = now, 205 = LGM)")
	eraFlag := flag.String("era", "", `(deprecated) named era: "now" or "205kya" — sets --kya if --kya is unset`)
	flag.Parse()

	if *eraFlag != "" {
		era, err := world.ParseEra(*eraFlag)
		if err != nil {
			log.Fatalf("%v", err)
		}
		// Only let --era override --kya if --kya wasn't explicitly provided.
		if !flagPassed("kya") {
			*kya = era.Kya()
		}
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

	w := world.Generate(*seed, *kya)
	if err := world.Persist(context.Background(), conn, w); err != nil {
		log.Fatalf("persist: %v", err)
	}

	fmt.Printf("seed=%d kya=%d era=%s  regions=%d rivers=%d  written to %s\n",
		*seed, w.Kya, w.Era, len(w.Regions), len(w.Rivers), *dbPath)
}

func flagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
