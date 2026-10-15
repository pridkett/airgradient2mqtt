package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TimescaleDB settings. The connection comes from the standard PG*
// environment variables (PGHOST, PGUSER, PGPASSWORD, PGDATABASE, ...), so
// the config file holds no credentials.
type tomlConfigTimescale struct {
	Table string
	// MaxQueue caps the readings held while the database is unreachable.
	MaxQueue int
}

// pgWriter inserts each reading as one row of a wide table. Readings that
// can't be written are queued, oldest first, and retried with the next one.
type pgWriter struct {
	pool     *pgxpool.Pool
	table    string
	columns  []string
	maxQueue int
	queue    [][]any
	checked  bool
}

func newPGWriter(ctx context.Context, cfg tomlConfigTimescale) (*pgWriter, error) {
	if cfg.Table == "" {
		return nil, fmt.Errorf("timescale: table is required")
	}
	pgcfg, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("timescale: %w", err)
	}
	pgcfg.ConnConfig.RuntimeParams["application_name"] = "airgradient2mqtt"
	pgcfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, pgcfg)
	if err != nil {
		return nil, fmt.Errorf("timescale: %w", err)
	}
	maxQueue := cfg.MaxQueue
	if maxQueue <= 0 {
		maxQueue = 1000 // over a day at the default 120 s poll
	}
	return &pgWriter{
		pool:     pool,
		table:    cfg.Table,
		columns:  append([]string{"time"}, pgColumns(reflect.TypeOf(airGradientStatus{}))...),
		maxQueue: maxQueue,
	}, nil
}

// pgColumns lists the struct's pg-tagged columns in field order.
func pgColumns(t reflect.Type) []string {
	var cols []string
	for i := 0; i < t.NumField(); i++ {
		if name := t.Field(i).Tag.Get("pg"); name != "" && name != "-" {
			cols = append(cols, name)
		}
	}
	return cols
}

// pgRow is a reading's values in pgColumns order, after its timestamp.
func pgRow(t time.Time, status any) []any {
	v := reflect.Indirect(reflect.ValueOf(status))
	row := []any{t}
	for i := 0; i < v.NumField(); i++ {
		if name := v.Type().Field(i).Tag.Get("pg"); name != "" && name != "-" {
			row = append(row, v.Field(i).Interface())
		}
	}
	return row
}

// check verifies the table has every column, so a schema mismatch fails
// loudly instead of losing readings.
func (w *pgWriter) check(ctx context.Context) error {
	rows, err := w.pool.Query(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1`, w.table)
	if err != nil {
		return err
	}
	have, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if len(have) == 0 {
		return fmt.Errorf("%w: table %q doesn't exist", errSchema, w.table)
	}
	got := map[string]bool{}
	for _, c := range have {
		got[c] = true
	}
	var missing []string
	for _, c := range w.columns {
		if !got[c] {
			missing = append(missing, c)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: table %q is missing columns %s", errSchema, w.table, strings.Join(missing, ", "))
	}
	return nil
}

var errSchema = fmt.Errorf("schema mismatch")

// write queues a reading and flushes the queue. It returns the number of
// rows written; on error the rows stay queued for the next call.
func (w *pgWriter) write(ctx context.Context, t time.Time, status any) (int, error) {
	w.queue = append(w.queue, pgRow(t, status))
	if over := len(w.queue) - w.maxQueue; over > 0 {
		logger.Warnf("TimescaleDB queue full, dropping the %d oldest readings", over)
		w.queue = w.queue[over:]
	}
	if !w.checked {
		if err := w.check(ctx); err != nil {
			return 0, err
		}
		w.checked = true
	}
	n, err := w.pool.CopyFrom(ctx, pgx.Identifier{w.table}, w.columns, pgx.CopyFromRows(w.queue))
	if err != nil {
		return 0, err
	}
	w.queue = w.queue[:0]
	return int(n), nil
}
