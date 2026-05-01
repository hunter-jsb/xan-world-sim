# xan-world-sim

A terminal sim of a single continent across ~250,000 years of geological time.
Mountains, plateaus, shelves, glaciers, and seas all derive from a climate
state that's a function of `kya` (kiloyears before present). Scrub kya in the
TUI and watch the world evolve — Agraria emerges from the Brine, ice sheets
march south, then everything reverses.

## Run

```bash
goose -dir migrations sqlite3 world.db up    # one-time schema
go run ./cmd/seedgen --seed 42 --kya 0       # generate a world
go run ./cmd/sim                             # explore it
```

In the TUI:

| key | action |
|-----|--------|
| `]` `[` | step ±5ka |
| `}` `{` | step ±25ka |
| `r` | reroll seed at current kya |
| `e` | jump now ↔ LGM |
| `q` | quit |

`go run ./cmd/sim --print` for a one-shot headless render.

## Stack

Go + Bubble Tea (TUI) + lipgloss (color) + SQLite (`modernc.org/sqlite`) +
sqlc (typed queries) + goose (migrations). Tests in `internal/world/` pin
deterministic snapshot hashes — if you change a worldgen constant, expect
them to drift, and update them in the same commit.
