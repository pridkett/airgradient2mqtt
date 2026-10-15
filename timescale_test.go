package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/withmandala/go-log"
)

func TestMain(m *testing.M) {
	logger = log.New(os.Stderr)
	os.Exit(m.Run())
}

// The columns must match the table in ansible-config's environment.sql.j2.
var wantColumns = "wifi mac rco2 pm01 pm02 pm10 pm003_count atmp atmp_compensated rhum rhum_compensated " +
	"pm02_compensated tvoc_index tvoc_raw nox_index nox_raw boot_count led_mode firmware model aqi"

func TestPGColumns(t *testing.T) {
	got := strings.Join(pgColumns(reflect.TypeOf(airGradientStatus{})), " ")
	if got != wantColumns {
		t.Errorf("columns\n got %s\nwant %s", got, wantColumns)
	}
}

func TestPGRow(t *testing.T) {
	ts := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	s := &airGradientStatus{Wifi: -59, Serialno: "dc5475bacf4c", Rco2: 570, AtmpCompensated: 19.89,
		Boot: 66766, BootCount: 66766, LedMode: "co2", Firmware: "3.7.0", Model: "I-9PSL", AQI: 12}
	row := pgRow(ts, s)
	if len(row) != 1+len(strings.Fields(wantColumns)) {
		t.Fatalf("row has %d values", len(row))
	}
	cols := append([]string{"time"}, strings.Fields(wantColumns)...)
	got := map[string]any{}
	for i, c := range cols {
		got[c] = row[i]
	}
	for c, want := range map[string]any{"time": ts, "wifi": -59, "mac": "dc5475bacf4c", "rco2": 570.0,
		"atmp_compensated": 19.89, "boot_count": 66766, "led_mode": "co2", "firmware": "3.7.0", "aqi": 12} {
		if got[c] != want {
			t.Errorf("%s = %v, want %v", c, got[c], want)
		}
	}
}

// Without a database, readings queue up to MaxQueue, dropping the oldest.
func TestQueueWhileDown(t *testing.T) {
	t.Setenv("PGHOST", "127.0.0.1")
	t.Setenv("PGPORT", "1")
	t.Setenv("PGCONNECT_TIMEOUT", "1")
	w, err := newPGWriter(context.Background(), tomlConfigTimescale{Table: "airgradient", MaxQueue: 3})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if _, err := w.write(context.Background(), time.Unix(int64(i), 0), &airGradientStatus{Rco2: float64(i)}); err == nil {
			t.Fatal("write succeeded with no database")
		}
	}
	if len(w.queue) != 3 || w.queue[0][0] != time.Unix(2, 0) {
		t.Errorf("queue = %v", w.queue)
	}
}

// TestTimescale runs against a real database when PGHOST is set, e.g. a
// local timescale/timescaledb container.
func TestTimescale(t *testing.T) {
	if os.Getenv("PGHOST") == "" {
		t.Skip("PGHOST not set")
	}
	ctx := context.Background()
	w, err := newPGWriter(ctx, tomlConfigTimescale{Table: "airgradient_test"})
	if err != nil {
		t.Fatal(err)
	}
	defer w.pool.Exec(ctx, `DROP TABLE IF EXISTS airgradient_test`)

	if _, err := w.write(ctx, time.Now(), &airGradientStatus{}); !errors.Is(err, errSchema) {
		t.Fatalf("missing table: err = %v", err)
	}
	w.queue = nil
	if _, err := w.pool.Exec(ctx, `CREATE TABLE airgradient_test (time timestamptz, mac text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.write(ctx, time.Now(), &airGradientStatus{}); !errors.Is(err, errSchema) || !strings.Contains(err.Error(), "rco2") {
		t.Fatalf("missing columns: err = %v", err)
	}
	w.queue = nil
	if _, err := w.pool.Exec(ctx, `DROP TABLE airgradient_test`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.pool.Exec(ctx, testSchema); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	n, err := w.write(ctx, ts, &airGradientStatus{Serialno: "dc5475bacf4c", Rco2: 658.33, AtmpCompensated: 19.89, LedMode: "co2"})
	if err != nil || n != 1 {
		t.Fatalf("write = %d, %v", n, err)
	}
	var mac, led string
	var rco2 float64
	var atmp float64
	var got time.Time
	err = w.pool.QueryRow(ctx, `SELECT time, mac, rco2, atmp_compensated, led_mode FROM airgradient_test`).Scan(&got, &mac, &rco2, &atmp, &led)
	if err != nil || !got.Equal(ts) || mac != "dc5475bacf4c" || rco2 != 658.33 || atmp != 19.89 || led != "co2" {
		t.Errorf("row = %v %s %v %v %s, %v", got, mac, rco2, atmp, led, err)
	}
}

const testSchema = `CREATE TABLE airgradient_test (
  time timestamptz NOT NULL, mac text NOT NULL, model text, firmware text, wifi integer,
  rco2 double precision, pm01 double precision, pm02 double precision, pm02_compensated double precision,
  pm10 double precision, pm003_count double precision, atmp double precision, atmp_compensated double precision,
  rhum double precision, rhum_compensated double precision, tvoc_index double precision, tvoc_raw double precision,
  nox_index double precision, nox_raw double precision, aqi integer, boot_count integer, led_mode text)`

// Firmware 3.x JSON, captured 2026-10-15. Decimal readings used to decode
// into ints and come out 0.
const firmware3JSON = `{"pm01":7.83,"pm02":11.67,"pm10":11.67,"pm003Count":1335.5,"pm02Compensated":6.52,
"atmp":19.77,"atmpCompensated":19.77,"rhum":62.04,"rhumCompensated":62.04,"rco2":658.33,"tvocIndex":48,
"tvocRaw":32182.08,"noxIndex":1,"noxRaw":17726,"boot":66906,"bootCount":66906,"wifi":-61,"ledMode":"co2",
"serialno":"dc5475bacf4c","firmware":"3.7.0","model":"I-9PSL"}`

func TestDecodeFirmware3(t *testing.T) {
	var s airGradientStatus
	if err := json.Unmarshal([]byte(firmware3JSON), &s); err != nil {
		t.Fatal(err)
	}
	if s.Rco2 != 658.33 || s.Rhum != 62.04 || s.Pm02 != 11.67 || s.Pm003count != 1335.5 || s.TvocRaw != 32182.08 || s.BootCount != 66906 {
		t.Errorf("decoded %+v", s)
	}
}

func TestPM25toAQI(t *testing.T) {
	for pm, want := range map[float64]int{0: 0, 6.52: 27, 12.0: 50, 12.05: 50, 12.1: 51, 35.45: 100, 35.5: 101} {
		if got := PM25toAQI(pm); got != want {
			t.Errorf("PM25toAQI(%v) = %d, want %d", pm, got, want)
		}
	}
}
