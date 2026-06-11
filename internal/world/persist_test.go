package world

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"github.com/hunterjsb/xan-world-sim/internal/migrations"
)

// openMigratedDB returns an in-memory SQLite with all migrations
// applied — the same schema the sim runs against.
func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	goose.SetLogger(goose.NopLogger())
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	if err := goose.Up(conn, "."); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return conn
}

// TestRegionKinds_MatchMigrations pins the Go-side RegionID↔kind table
// to the regions table the migrations actually seed. If a migration
// adds or renames a region without updating regions.go (or vice
// versa), this fails.
func TestRegionKinds_MatchMigrations(t *testing.T) {
	conn := openMigratedDB(t)
	rows, err := conn.Query("SELECT id, kind FROM regions")
	if err != nil {
		t.Fatalf("query regions: %v", err)
	}
	defer rows.Close()
	dbKinds := make(map[int64]string)
	for rows.Next() {
		var id int64
		var kind string
		if err := rows.Scan(&id, &kind); err != nil {
			t.Fatalf("scan: %v", err)
		}
		dbKinds[id] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	for id, kind := range dbKinds {
		if got := RegionKind(id); got != kind {
			t.Errorf("RegionKind(%d) = %q, but migrations seed kind %q", id, got, kind)
		}
	}
	for id, kind := range kindByRegionID {
		if dbKind, ok := dbKinds[id]; !ok {
			t.Errorf("region %d (%q) in Go table but not seeded by migrations", id, kind)
		} else if dbKind != kind {
			t.Errorf("region %d: Go kind %q != migration kind %q", id, kind, dbKind)
		}
	}
}

// TestPersist_RoundTrip generates a world, persists it, and verifies
// every table holds exactly what the World struct holds. Runs Persist
// twice to confirm idempotency (regen overwrites cleanly).
func TestPersist_RoundTrip(t *testing.T) {
	conn := openMigratedDB(t)
	ctx := context.Background()
	w := Generate(42, KyaNow)

	for pass := 1; pass <= 2; pass++ {
		if err := Persist(ctx, conn, w); err != nil {
			t.Fatalf("persist (pass %d): %v", pass, err)
		}

		counts := []struct {
			table string
			want  int
		}{
			{"region_cells", len(w.Regions)},
			{"rivers", len(w.RiverInfo)},
			{"river_cells", len(w.Rivers)},
			{"seats", len(w.Seats)},
			{"lakes", len(w.Lakes)},
			{"passes", len(w.Passes)},
			{"roads", len(w.Roads)},
			{"road_cells", len(w.RoadCells)},
			{"dens", len(w.Dens)},
			{"drake_nests", len(w.Nests)},
			{"wyvern_rookeries", len(w.Rookeries)},
			{"realms", len(w.Realms)},
			{"territory", len(w.Territory)},
		}
		for _, c := range counts {
			var n int
			if err := conn.QueryRow("SELECT COUNT(*) FROM " + c.table).Scan(&n); err != nil {
				t.Fatalf("count %s: %v", c.table, err)
			}
			if n != c.want {
				t.Errorf("pass %d: %s has %d rows, want %d", pass, c.table, n, c.want)
			}
		}

		var seed, kya string
		if err := conn.QueryRow("SELECT value FROM world_meta WHERE key='seed'").Scan(&seed); err != nil {
			t.Fatalf("meta seed: %v", err)
		}
		if err := conn.QueryRow("SELECT value FROM world_meta WHERE key='kya'").Scan(&kya); err != nil {
			t.Fatalf("meta kya: %v", err)
		}
		if seed != "42" || kya != "0" {
			t.Errorf("pass %d: meta seed=%q kya=%q, want 42, 0", pass, seed, kya)
		}
	}

	// A world worth testing should actually have content.
	if len(w.Regions) == 0 || len(w.RiverInfo) == 0 || len(w.Seats) == 0 {
		t.Errorf("generated world is suspiciously empty: regions=%d rivers=%d seats=%d",
			len(w.Regions), len(w.RiverInfo), len(w.Seats))
	}

	// Spot-check a joined read: every seat tier must resolve to a
	// region kind via the FK the TUI's GetSeatsInBounds relies on.
	var orphans int
	err := conn.QueryRow(`
		SELECT COUNT(*) FROM seats s
		LEFT JOIN regions r ON r.id = s.tier_id
		WHERE r.id IS NULL`).Scan(&orphans)
	if err != nil {
		t.Fatalf("orphan check: %v", err)
	}
	if orphans != 0 {
		t.Errorf("%d seats reference a tier_id missing from regions", orphans)
	}
}
