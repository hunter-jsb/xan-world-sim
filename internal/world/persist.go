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

	if _, err := tx.ExecContext(ctx, "DELETE FROM river_cells"); err != nil {
		return fmt.Errorf("clear rivers: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM region_cells"); err != nil {
		return fmt.Errorf("clear regions: %w", err)
	}

	rcStmt, err := tx.PrepareContext(ctx,
		"INSERT INTO region_cells (region_id, x, y) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare region: %w", err)
	}
	defer rcStmt.Close()
	for _, c := range w.Regions {
		if _, err := rcStmt.ExecContext(ctx, c.RegionID, c.X, c.Y); err != nil {
			return fmt.Errorf("insert region cell: %w", err)
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
	if _, err := metaStmt.ExecContext(ctx, "seed", fmt.Sprint(w.Seed)); err != nil {
		return fmt.Errorf("set seed: %w", err)
	}
	if _, err := metaStmt.ExecContext(ctx, "generated_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("set generated_at: %w", err)
	}

	return tx.Commit()
}
