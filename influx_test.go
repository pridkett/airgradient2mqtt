package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// InfluxDB rejects a field whose type changes, so readings that are now
// float64 must still be written as the integers the measurement has.
func TestInfluxKeepsIntegerFields(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	config.Influx = tomlConfigInflux{Hostname: u.Hostname(), Port: port, Database: "airgradient"}

	var s airGradientStatus
	if err := json.Unmarshal([]byte(firmware3JSON), &s); err != nil {
		t.Fatal(err)
	}
	publishInflux(&s, "airgradient", map[string]string{"mac": s.Serialno})
	for _, want := range []string{"rco2=658i", "rhum=62i", "pm02=12i", "pm003_count=1336i", "atmp=19.77,", "led_mode=\"co2\""} {
		if !strings.Contains(body, want) {
			t.Errorf("line protocol is missing %s:\n%s", want, body)
		}
	}
}
