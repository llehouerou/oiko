// Package api exposes Home over HTTP: one SSE stream for Updates and the
// Status of each module's Releases, one JSON endpoint for Commands, the
// Automations' documents, Traces and the Command history, what Oiko is built
// from, and the static web client.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"time"

	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
	"github.com/llehouerou/oiko/internal/release"
)

// ponytail: no authentication yet; anyone on the LAN can observe and command.
// Add accounts before exposing Oiko beyond a trusted network.
// b is what Oiko is built from, install its Install (see CONTEXT.md), releases
// what is newer, bridges the type of each Bridge of the configuration, by name,
// public the Public URL, nil when the configuration has none: the origin and
// RP ID sign-in will check.
func Handler(h *home.Home, automations *automation.Engine, hist *history.Store, b build.Build, install string, releases *release.Checker, bridges map[string]string, public *url.URL, static fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/updates", updates(h, releases))
	mux.HandleFunc("GET /api/build", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			build.Build
			Install string            `json:"install"`
			Bridges map[string]string `json:"bridges"`
		}{b, install, bridges})
	})
	mux.HandleFunc("POST /api/commands", command(h))
	mux.HandleFunc("PATCH /api/devices/{id}", rename(h))
	mux.HandleFunc("PUT /api/icon", setIcon(h))
	mux.HandleFunc("PUT /api/area", setArea(h))
	mux.HandleFunc("POST /api/areas", defineArea(h))
	mux.HandleFunc("PUT /api/areas/{id}", defineArea(h))
	// Its Area Aggregates go with it, and their History: Home announces it.
	mux.HandleFunc("DELETE /api/areas/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.DeleteArea(home.AreaID(r.PathValue("id"))))
	})
	// How the dashboard shows Area {id}: the tiles it hides, the kinds of Area
	// Aggregates its header leaves out.
	mux.HandleFunc("PUT /api/areas/{id}/display", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hidden           []home.Target `json:"hidden"`
			HiddenAggregates []string      `json:"hiddenAggregates"`
		}
		if decode(w, r, &req) {
			reply(w, h.SetAreaDisplay(home.AreaID(r.PathValue("id")), req.Hidden, req.HiddenAggregates))
		}
	})
	// Area {id}'s grid on the dashboard: its columns and where its tiles sit.
	mux.HandleFunc("PUT /api/areas/{id}/layout", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Columns int              `json:"columns"`
			Layout  []home.Placement `json:"layout"`
		}
		if decode(w, r, &req) {
			reply(w, h.SetAreaLayout(home.AreaID(r.PathValue("id")), req.Columns, req.Layout))
		}
	})
	mux.HandleFunc("PUT /api/areas", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Order []home.AreaID `json:"order"`
		}
		if decode(w, r, &req) {
			reply(w, h.OrderAreas(req.Order))
		}
	})
	mux.HandleFunc("DELETE /api/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.Delete(home.DeviceID(r.PathValue("id"))))
	})
	mux.HandleFunc("POST /api/devices/{id}/replace", replace(h))
	mux.HandleFunc("POST /api/aggregates", defineAggregate(h))
	mux.HandleFunc("PUT /api/aggregates/{id}", defineAggregate(h))
	mux.HandleFunc("DELETE /api/aggregates/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.DeleteAggregate(home.AggregateID(r.PathValue("id"))))
	})
	mux.HandleFunc("POST /api/flags", defineFlag(h))
	mux.HandleFunc("PUT /api/flags/{id}", defineFlag(h))
	mux.HandleFunc("DELETE /api/flags/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.DeleteFlag(home.FlagID(r.PathValue("id"))))
	})
	mux.HandleFunc("GET /api/automations", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, automations.Documents())
	})
	mux.HandleFunc("POST /api/automations", defineAutomation(automations))
	mux.HandleFunc("PUT /api/automations/{id}", defineAutomation(automations))
	mux.HandleFunc("DELETE /api/automations/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, automations.Delete(r.PathValue("id")))
	})
	// Runs an Automation from its Manual trigger step, at once: how the Run ended.
	mux.HandleFunc("POST /api/automations/{id}/steps/{step}/run", func(w http.ResponseWriter, r *http.Request) {
		end, err := automations.Trigger(r.PathValue("id"), r.PathValue("step"))
		respond(w, end, err)
	})
	mux.HandleFunc("GET /api/automations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := hist.Runs(r.PathValue("id"))
		respond(w, runs, err)
	})
	mux.HandleFunc("DELETE /api/automations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		reply(w, hist.ClearRuns(r.PathValue("id")))
	})
	mux.HandleFunc("GET /api/automations/{id}/state", func(w http.ResponseWriter, r *http.Request) {
		state, err := automations.State(r.PathValue("id"))
		respond(w, state, err)
	})
	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		trace, err := hist.Trace(r.PathValue("id"))
		respond(w, trace, err)
	})
	mux.HandleFunc("DELETE /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, hist.DeleteRun(r.PathValue("id")))
	})
	// A Target's latest Commands: ?target=<key>.
	mux.HandleFunc("GET /api/commands", func(w http.ResponseWriter, r *http.Request) {
		t, err := home.ParseTarget(r.URL.Query().Get("target"))
		if err != nil {
			reply(w, err)
			return
		}
		commands, err := hist.Commands(t)
		respond(w, commands, err)
	})
	mux.HandleFunc("POST /api/history", readHistory(h, hist))
	mux.HandleFunc("POST /api/history/periods", readPeriods(h, hist))
	mux.HandleFunc("GET /api/lost-entries", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]uint64{"lost": hist.Lost()})
	})
	mux.Handle("GET /", http.FileServerFS(static))
	return http.NewCrossOriginProtection().Handler(mux)
}

// respond answers v in JSON, or the status matching err.
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		reply(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// updates streams a snapshot, then every Update, as Server-Sent Events; the
// Status of each module's Releases comes with the snapshot, and again, as a
// "releases" message, each time it changes. Updates available at once are
// written together and flushed once.
func updates(h *home.Home, releases *release.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap, ch, cancel := h.Subscribe()
		defer cancel()
		statuses, changed := releases.Statuses()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		rc := http.NewResponseController(w)

		if writeEvent(w, struct {
			Kind string `json:"kind"`
			home.Snapshot
			Releases []release.Status `json:"releases"`
		}{"snapshot", snap, statuses}) != nil || rc.Flush() != nil {
			return
		}
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		for {
			select {
			case u, ok := <-ch:
				// Closed: this observer fell too far behind. Ending the stream makes the
				// browser reconnect and start over from a fresh snapshot.
				if !ok || !writeBatch(w, u, ch) || rc.Flush() != nil {
					return
				}
			case <-changed:
				statuses, changed = releases.Statuses()
				if writeEvent(w, struct {
					Kind     string           `json:"kind"`
					Releases []release.Status `json:"releases"`
				}{"releases", statuses}) != nil || rc.Flush() != nil {
					return
				}
			case <-keepalive.C:
				if _, err := w.Write([]byte(": keepalive\n\n")); err != nil || rc.Flush() != nil {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

// writeBatch writes u and every Update already queued behind it; it reports
// false if the stream must end.
func writeBatch(w http.ResponseWriter, u home.Update, ch <-chan home.Update) bool {
	for {
		if writeEvent(w, u) != nil {
			return false
		}
		select {
		case next, ok := <-ch:
			if !ok {
				return false
			}
			u = next
		default:
			return true
		}
	}
}

func writeEvent(w http.ResponseWriter, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(append([]byte("data: "), data...), '\n', '\n'))
	return err
}

// command addresses a Target by its key.
func command(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target     home.Target    `json:"target"`
			Values     map[string]any `json:"values"`
			Transition float64        `json:"transition"` // fade duration, in seconds
		}
		if !decode(w, r, &req) {
			return
		}
		id, err := h.Command(req.Target, home.Request{Values: req.Values, Transition: time.Duration(req.Transition * float64(time.Second))})
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"id": id})
	}
}

func rename(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if decode(w, r, &req) {
			reply(w, h.Rename(home.DeviceID(r.PathValue("id")), req.Name))
		}
	}
}

// setIcon sets a Device's or an Aggregate's Icon; "" for the default.
func setIcon(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target home.Target `json:"target"`
			Icon   string      `json:"icon"`
		}
		if decode(w, r, &req) {
			reply(w, h.SetIcon(req.Target, req.Icon))
		}
	}
}

// setArea assigns an Area to a Device, a Function, a Flag or an Aggregate;
// "" for none, or a Function's Device's.
func setArea(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Target home.Target `json:"target"`
			Area   home.AreaID `json:"area"`
		}
		if decode(w, r, &req) {
			reply(w, h.SetArea(req.Target, req.Area))
		}
	}
}

// defineArea creates an Area, or renames Area {id} when there is one.
func defineArea(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &req) {
			return
		}
		if id := r.PathValue("id"); id != "" {
			reply(w, h.RenameArea(home.AreaID(id), req.Name))
			return
		}
		_, err := h.CreateArea(req.Name)
		reply(w, err)
	}
}

// replace attaches the hardware of Device "with" to the Detached Device {id};
// its History follows, as Home announces it.
func replace(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			With home.DeviceID `json:"with"`
		}
		if decode(w, r, &req) {
			reply(w, h.Replace(home.DeviceID(r.PathValue("id")), req.With))
		}
	}
}

// readHistory answers the History of refs from from to to, Unix ms, in at
// most points points per series. A ref to capability "" is its Target's
// Availability. With markers, the Commands and Runs over the range come too.
func readHistory(h *home.Home, hist *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			From    int64      `json:"from"`
			To      int64      `json:"to"`
			Points  int        `json:"points"`
			Markers bool       `json:"markers"`
			Refs    []home.Ref `json:"refs"`
		}
		if !decode(w, r, &req) {
			return
		}
		const maxPoints = 10000 // within a few screens
		from, to, err := span(req.From, req.To)
		if err == nil && (req.Points < 1 || req.Points > maxPoints) {
			err = fmt.Errorf("%w: 1 to %d points", home.ErrInvalid, maxPoints)
		}
		if err != nil {
			reply(w, err)
			return
		}
		q := history.Query{From: from, To: to, Points: req.Points, Markers: req.Markers}
		for _, ref := range req.Refs {
			x, err := series(h, ref)
			if err != nil {
				reply(w, err)
				return
			}
			q.Series = append(q.Series, x)
		}
		a, err := hist.Points(q)
		respond(w, a, err)
	}
}

// readPeriods answers what items sum up per hour, day, week or month, or over
// the span, from from to to, Unix ms.
func readPeriods(h *home.Home, hist *history.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			From  int64  `json:"from"`
			To    int64  `json:"to"`
			Per   string `json:"per"`
			Items []struct {
				Ref     home.Ref        `json:"ref"`
				Measure history.Measure `json:"measure"`
			} `json:"items"`
		}
		if !decode(w, r, &req) {
			return
		}
		from, to, err := span(req.From, req.To)
		if err != nil {
			reply(w, err)
			return
		}
		q := history.PeriodsQuery{From: from, To: to, Per: req.Per}
		for _, it := range req.Items {
			x, err := series(h, it.Ref)
			if err != nil {
				reply(w, err)
				return
			}
			item := history.Item{Series: x, Measure: it.Measure}
			// An Aggregate's energy is its current members', never its own sum's.
			if it.Measure == history.Energy && it.Ref.Target.Aggregate() != "" {
				item.Members = []history.Series{}
				for _, m := range h.Members(it.Ref.Target) {
					x, err := series(h, m.Ref(it.Ref.Capability))
					if err != nil {
						reply(w, err)
						return
					}
					item.Members = append(item.Members, x)
				}
			}
			q.Items = append(q.Items, item)
		}
		a, err := hist.Periods(q)
		respond(w, a, err)
	}
}

// span is [from, to], Unix ms, as times: from before to, within what Unix ns
// hold.
func span(from, to int64) (time.Time, time.Time, error) {
	const maxMs = math.MaxInt64 / int64(time.Millisecond)
	if to <= from || from < -maxMs || to > maxMs {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: a range needs from before to", home.ErrInvalid)
	}
	return time.UnixMilli(from), time.UnixMilli(to), nil
}

// series is how History reads ref: a ref to capability "" is its Target's
// Availability.
func series(h *home.Home, ref home.Ref) (history.Series, error) {
	c, err := h.Capability(ref)
	if err != nil {
		return history.Series{}, fmt.Errorf("%s %q: %w", ref.Target, ref.Capability, err)
	}
	return history.Series{Ref: ref, Type: c.Type, Stateless: c.Stateless, Counter: c.Counter}, nil
}

// defineAggregate creates an Aggregate, or replaces the definition of
// Aggregate {id} when there is one.
func defineAggregate(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name    string           `json:"name"`
			Members []home.Target    `json:"members"`
			Binary  home.BinaryRule  `json:"binary"`
			Numeric home.NumericRule `json:"numeric"`
		}
		if !decode(w, r, &req) {
			return
		}
		if id := r.PathValue("id"); id != "" {
			reply(w, h.EditAggregate(home.AggregateID(id), req.Name, req.Members, req.Binary, req.Numeric))
			return
		}
		_, err := h.CreateAggregate(req.Name, req.Members, req.Binary, req.Numeric)
		reply(w, err)
	}
}

// defineFlag creates a Flag, or renames Flag {id} when there is one.
func defineFlag(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if !decode(w, r, &req) {
			return
		}
		if id := r.PathValue("id"); id != "" {
			reply(w, h.RenameFlag(home.FlagID(id), req.Name))
			return
		}
		_, err := h.CreateFlag(req.Name)
		reply(w, err)
	}
}

// defineAutomation creates an Automation, answering its id, or replaces
// Automation {id} when there is one. Its own id is ignored.
func defineAutomation(e *automation.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var doc automation.Document
		if !decode(w, r, &doc) {
			return
		}
		if id := r.PathValue("id"); id != "" {
			reply(w, e.Replace(id, doc))
			return
		}
		id, err := e.Create(doc)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(v); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// reply answers 204 on success, or the status matching a Home error.
func reply(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, home.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, home.ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, home.ErrBridgeOffline):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, home.ErrNotRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
