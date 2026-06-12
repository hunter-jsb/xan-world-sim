package world

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Persist writes the world to the database in a single transaction:
// truncates all world tables, inserts everything fresh, records the
// seed and climate state in world_meta. Idempotent.
func Persist(ctx context.Context, conn *sql.DB, w World) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// SQLite FKs are off by default so "DELETE rivers cascades" can't
	// be relied on. Be explicit, child tables before their parents.
	for _, table := range []string{
		"river_cells", "rivers", "region_cells", "seats", "lakes",
		"passes", "road_cells", "roads", "dens", "drake_nests",
		"wyvern_rookeries", "territory", "realms",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}

	if err := insertAll(ctx, tx, "region cell",
		"INSERT INTO region_cells (region_id, x, y, elevation, drainage) VALUES (?, ?, ?, ?, ?)",
		w.Regions, func(c RegionCell) []any { return []any{c.RegionID, c.X, c.Y, c.Elevation, c.Drainage} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "river info",
		"INSERT INTO rivers (id, name, drainage) VALUES (?, ?, ?)",
		w.RiverInfo, func(r River) []any { return []any{r.ID, r.Name, r.Drainage} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "river cell",
		"INSERT INTO river_cells (river_id, x, y, ord) VALUES (?, ?, ?, ?)",
		w.Rivers, func(r RiverCell) []any { return []any{r.RiverID, r.X, r.Y, r.Ord} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "realm",
		"INSERT INTO realms (id, name, is_crown, seat_x, seat_y) VALUES (?, ?, ?, ?, ?)",
		w.Realms, func(r Realm) []any { return []any{r.ID, r.Name, r.IsCrown, r.SeatX, r.SeatY} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "seat",
		"INSERT INTO seats (x, y, tier_id, name, pressure, realm_id, allegiance) VALUES (?, ?, ?, ?, ?, ?, ?)",
		w.Seats, func(s NamedSeat) []any {
			return []any{s.X, s.Y, s.Tier, s.Name, s.Pressure, s.RealmID, s.Allegiance}
		},
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "territory",
		"INSERT INTO territory (x, y, realm_id) VALUES (?, ?, ?)",
		w.Territory, func(tc TerritoryCell) []any { return []any{tc.X, tc.Y, tc.RealmID} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "lake",
		"INSERT INTO lakes (id, name, x, y, surface_elev, max_depth) VALUES (?, ?, ?, ?, ?, ?)",
		w.Lakes, func(l LakeInfo) []any {
			return []any{l.ID, l.Name, l.X, l.Y, l.SurfaceElev, l.MaxDepth}
		},
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "pass",
		"INSERT INTO passes (id, name, x, y) VALUES (?, ?, ?, ?)",
		w.Passes, func(p PassInfo) []any { return []any{p.ID, p.Name, p.X, p.Y} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "road",
		"INSERT INTO roads (id, from_x, from_y, to_x, to_y) VALUES (?, ?, ?, ?, ?)",
		w.Roads, func(r Road) []any { return []any{r.ID, r.FromX, r.FromY, r.ToX, r.ToY} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "road cell",
		"INSERT INTO road_cells (road_id, x, y, ord) VALUES (?, ?, ?, ?)",
		w.RoadCells, func(rc RoadCell) []any { return []any{rc.RoadID, rc.X, rc.Y, rc.Ord} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "den",
		"INSERT INTO dens (id, name, x, y, elevation) VALUES (?, ?, ?, ?, ?)",
		w.Dens, func(d DenInfo) []any { return []any{d.ID, d.Name, d.X, d.Y, d.Elevation} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "nest",
		"INSERT INTO drake_nests (id, name, x, y, elevation) VALUES (?, ?, ?, ?, ?)",
		w.Nests, func(n NestInfo) []any { return []any{n.ID, n.Name, n.X, n.Y, n.Elevation} },
	); err != nil {
		return err
	}
	if err := insertAll(ctx, tx, "rookery",
		"INSERT INTO wyvern_rookeries (id, name, x, y, elevation) VALUES (?, ?, ?, ?, ?)",
		w.Rookeries, func(r RookeryInfo) []any { return []any{r.ID, r.Name, r.X, r.Y, r.Elevation} },
	); err != nil {
		return err
	}

	era := string(w.Era)
	if era == "" {
		era = string(EraForKya(w.Kya))
	}
	meta := []struct{ k, v string }{
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
	if err := insertAll(ctx, tx, "meta",
		"INSERT OR REPLACE INTO world_meta (key, value) VALUES (?, ?)",
		meta, func(p struct{ k, v string }) []any { return []any{p.k, p.v} },
	); err != nil {
		return err
	}

	return tx.Commit()
}

// insertAll prepares query once and executes it for every row, mapping
// each through args. label only feeds error messages.
func insertAll[T any](ctx context.Context, tx *sql.Tx, label, query string, rows []T, args func(T) []any) error {
	if len(rows) == 0 {
		return nil
	}
	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("prepare %s: %w", label, err)
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, args(r)...); err != nil {
			return fmt.Errorf("insert %s: %w", label, err)
		}
	}
	return nil
}
