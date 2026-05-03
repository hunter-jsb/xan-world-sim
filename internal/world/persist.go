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
	if _, err := tx.ExecContext(ctx, "DELETE FROM seats"); err != nil {
		return fmt.Errorf("clear seats: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM lakes"); err != nil {
		return fmt.Errorf("clear lakes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM passes"); err != nil {
		return fmt.Errorf("clear passes: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM road_cells"); err != nil {
		return fmt.Errorf("clear road_cells: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM roads"); err != nil {
		return fmt.Errorf("clear roads: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM dens"); err != nil {
		return fmt.Errorf("clear dens: %w", err)
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
		"INSERT INTO rivers (id, name, drainage) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare river info: %w", err)
	}
	defer riverStmt.Close()
	for _, r := range w.RiverInfo {
		if _, err := riverStmt.ExecContext(ctx, r.ID, r.Name, r.Drainage); err != nil {
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

	seatStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO seats (x, y, tier_id, name, pressure) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare seat: %w", err)
	}
	defer seatStmt.Close()
	for _, s := range w.Seats {
		if _, err := seatStmt.ExecContext(ctx, s.X, s.Y, s.Tier, s.Name, s.Pressure); err != nil {
			return fmt.Errorf("insert seat: %w", err)
		}
	}

	lakeStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO lakes (id, name, x, y) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare lake: %w", err)
	}
	defer lakeStmt.Close()
	for _, l := range w.Lakes {
		if _, err := lakeStmt.ExecContext(ctx, l.ID, l.Name, l.X, l.Y); err != nil {
			return fmt.Errorf("insert lake: %w", err)
		}
	}

	passStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO passes (id, name, x, y) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare pass: %w", err)
	}
	defer passStmt.Close()
	for _, p := range w.Passes {
		if _, err := passStmt.ExecContext(ctx, p.ID, p.Name, p.X, p.Y); err != nil {
			return fmt.Errorf("insert pass: %w", err)
		}
	}

	roadStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO roads (id, from_x, from_y, to_x, to_y) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare road: %w", err)
	}
	defer roadStmt.Close()
	for _, r := range w.Roads {
		if _, err := roadStmt.ExecContext(ctx, r.ID, r.FromX, r.FromY, r.ToX, r.ToY); err != nil {
			return fmt.Errorf("insert road: %w", err)
		}
	}

	rcStmt2, err := tx.PrepareContext(ctx,
		"INSERT INTO road_cells (road_id, x, y, ord) VALUES (?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare road_cell: %w", err)
	}
	defer rcStmt2.Close()
	for _, rc := range w.RoadCells {
		if _, err := rcStmt2.ExecContext(ctx, rc.RoadID, rc.X, rc.Y, rc.Ord); err != nil {
			return fmt.Errorf("insert road_cell: %w", err)
		}
	}

	denStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO dens (id, name, x, y, elevation) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare den: %w", err)
	}
	defer denStmt.Close()
	for _, d := range w.Dens {
		if _, err := denStmt.ExecContext(ctx, d.ID, d.Name, d.X, d.Y, d.Elevation); err != nil {
			return fmt.Errorf("insert den: %w", err)
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
