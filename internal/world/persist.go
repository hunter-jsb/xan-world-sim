package world

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Persist writes the world to the database in a single transaction:
// truncates region_cells + river_cells, inserts everything fresh,
// records the seed in world_meta. Idempotent.
func Persist(ctx context.Context, conn *sql.DB, w World) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// SQLite FKs are off by default so "DELETE rivers cascades" can't
	// be relied on. Be explicit: drain river_cells first, then rivers.
	if _, err := tx.ExecContext(ctx, "DELETE FROM river_cells"); err != nil {
		return fmt.Errorf("clear river_cells: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM rivers"); err != nil {
		return fmt.Errorf("clear rivers: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM region_cells"); err != nil {
		return fmt.Errorf("clear regions: %w", err)
	}

	rcStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO region_cells (region_id, x, y, elevation) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare region: %w", err)
	}
	defer rcStmt.Close()
	for _, c := range w.Regions {
		if _, err := rcStmt.ExecContext(ctx, c.RegionID, c.X, c.Y, c.Elevation); err != nil {
			return fmt.Errorf("insert region cell: %w", err)
		}
	}

	riverStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO rivers (id, name) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare river info: %w", err)
	}
	defer riverStmt.Close()
	for _, r := range w.RiverInfo {
		if _, err := riverStmt.ExecContext(ctx, r.ID, r.Name); err != nil {
			return fmt.Errorf("insert river info: %w", err)
		}
	}

	rvStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO river_cells (river_id, x, y, ord) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare river: %w", err)
	}
	defer rvStmt.Close()
	for _, r := range w.Rivers {
		if _, err := rvStmt.ExecContext(ctx, r.RiverID, r.X, r.Y, r.Ord); err != nil {
			return fmt.Errorf("insert river cell: %w", err)
		}
	}

	metaStmt, err := tx.PrepareContext(ctx,
		"INSERT OR REPLACE INTO world_meta (key, value) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare meta: %w", err)
	}
	defer metaStmt.Close()
	era := string(w.Era)
	if era == "" {
		era = string(EraForKya(w.Kya))
	}
	pairs := []struct{ k, v string }{
		{"seed", fmt.Sprint(w.Seed)},
		{"kya", fmt.Sprint(w.Kya)},
		{"era", era},
		{"lat_top", fmt.Sprintf("%g", w.LatTop)},
		{"lat_bottom", fmt.Sprintf("%g", w.LatBottom)},
		{"obliquity", fmt.Sprintf("%g", w.Orbital.Obliquity)},
		{"eccentricity", fmt.Sprintf("%g", w.Orbital.Eccentricity)},
		{"precession", fmt.Sprintf("%g", w.Orbital.Precession)},
		{"sea_level_delta", fmt.Sprintf("%g", w.Climate.SeaLevelDelta)},
		{"glacial_index", fmt.Sprintf("%g", w.Climate.GlacialIndex)},
		{"global_mean_temp_delta", fmt.Sprintf("%g", w.Climate.GlobalMeanTempDelta)},
		{"generated_at", time.Now().UTC().Format(time.RFC3339)},
	}
	for _, p := range pairs {
		if _, err := metaStmt.ExecContext(ctx, p.k, p.v); err != nil {
			return fmt.Errorf("set %s: %w", p.k, err)
		}
	}

	return tx.Commit()
}
