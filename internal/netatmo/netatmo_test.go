package netatmo

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/llehouerou/oiko/internal/home"
	"github.com/llehouerou/oiko/internal/store"
)

// A failed poll is tried again soon, then less and less often, never less
// often than a poll.
func TestBackoff(t *testing.T) {
	var waits []time.Duration
	for w := time.Duration(0); len(waits) < 7; {
		w = backoff(w)
		waits = append(waits, w)
	}
	want := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, pollEvery, pollEvery}
	for i := range want {
		if waits[i] != want[i] {
			t.Fatalf("waits = %v, want %v", waits, want)
		}
	}
}

// TestPollFeedsHome polls a getstationsdata response recorded on a real
// account, its identifiers and names replaced, through a token refresh that
// rotates the refresh token.
func TestPollFeedsHome(t *testing.T) {
	recorded, err := os.ReadFile("testdata/getstationsdata.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("refresh_token") != "r1" || r.FormValue("client_secret") != "secret" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"a2","refresh_token":"r2","expires_in":10800}`))
	})
	mux.HandleFunc("GET /api/getstationsdata", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer a2" {
			http.Error(w, `{"error":{"code":2,"message":"Invalid access token"}}`, http.StatusForbidden)
			return
		}
		w.Write(recorded)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tokenPath := filepath.Join(t.TempDir(), "netatmo-token.json")
	b := newBridge(srv.URL, "id", "secret", &oauth2.Token{RefreshToken: "r1"}, tokenPath, slog.Default())
	h := home.New(nil, nil, nil, nil, nil, nil, nil, nil, nil)
	b.port = h.Attach("netatmo", b)
	stations, err := b.fetch(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var saved oauth2.Token
	if err := store.Load(tokenPath, tokenFormat, &saved); err != nil || saved.RefreshToken != "r2" {
		t.Fatalf("saved token %+v, %v: want the rotated refresh token", saved, err)
	}
	b.update(stations)
	select {
	case <-h.Known():
	default:
		t.Fatal("not replayed after the first poll")
	}

	s, _, cancel := h.Subscribe()
	defer cancel()
	if !s.Bridges["netatmo"] || len(s.Devices) != 5 {
		t.Fatalf("online %v, %d devices", s.Bridges["netatmo"], len(s.Devices))
	}
	ids := map[string]home.DeviceID{}
	for _, d := range s.Devices {
		ids[d.NativeAddress] = d.ID
	}
	values := map[home.Ref]home.Value{}
	for _, v := range s.Values {
		values[v.Ref] = v.Value
	}
	indoor := home.TargetDevice(ids["70:ee:50:00:00:01"], "co2").Ref("co2")
	for ref, want := range map[home.Ref]any{
		indoor: 518.0,
		home.TargetDevice(ids["70:ee:50:00:00:01"], "pressure").Ref("pressure"):       1021.7,
		home.TargetDevice(ids["02:00:00:00:00:05"], "temperature").Ref("temperature"): 23.7,
		home.TargetDevice(ids["02:00:00:00:00:05"], "").Ref("battery"):                32.0,
		home.TargetDevice(ids["03:00:00:00:00:03"], "").Ref("battery"):                nil, // unreachable: stale
	} {
		if got := values[ref].Data; got != want {
			t.Errorf("%v = %v, want %v", ref, got, want)
		}
	}
	if at := values[indoor].At; !at.Equal(time.Unix(1790877590, 0)) {
		t.Errorf("measured at %v, want the station's time_utc", at)
	}
	for addr, want := range map[string]home.Availability{"03:00:00:00:00:02": home.Online, "05:00:00:00:00:04": home.Offline} {
		if got := s.Availability[home.TargetDevice(ids[addr], "")]; got != want {
			t.Errorf("%s: %s, want %s", addr, got, want)
		}
	}
}
