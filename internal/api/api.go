// Package api exposes Home over HTTP: one SSE stream for Updates and the
// Status of each module's Releases, one JSON endpoint for Commands, the
// Automations' documents, Traces and the Command history, what Oiko is built
// from, who is signed in, and the static web client.
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/automation"
	"github.com/llehouerou/oiko/internal/build"
	"github.com/llehouerou/oiko/internal/camera"
	"github.com/llehouerou/oiko/internal/dashboard"
	"github.com/llehouerou/oiko/internal/history"
	"github.com/llehouerou/oiko/internal/home"
	"github.com/llehouerou/oiko/internal/release"
)

// Handler serves the API to a Session or a Token, each endpoint to the
// Access levels it declares, and the web client to anyone (ADR 0023, 0027,
// 0034). cams are the home's cameras, acc keeps who signs in, dash the custom
// Dashboards, b is what Oiko is built from, install its Install (see
// CONTEXT.md), releases what is newer, bridges the type of each Bridge of the
// configuration, by name, public the Public URL, nil when the configuration
// has none: the origin sign-in checks.
func Handler(h *home.Home, automations *automation.Engine, hist *history.Store, cams *camera.Cameras, acc *access.Store, dash *dashboard.Store, b build.Build, install string, releases *release.Checker, bridges map[string]string, public *url.URL, static fs.FS) http.Handler {
	mux := http.NewServeMux()
	handle := func(level access.Level, pattern string, serve http.HandlerFunc) {
		mux.HandleFunc(pattern, needs(level, serve))
	}
	guest, member, admin := access.Guest, access.Member, access.Admin

	// A Guest observes the home as it is now, commands it and starts Manual
	// triggers, which the event stream carries as Automation statuses.
	handle(guest, "GET /api/updates", updates(h, acc, dash, releases))
	handle(guest, "POST /api/commands", command(h))
	// A Person's own Dashboards, a Guest's too, and an Admin's shared ones; the
	// dashboards module refuses a Kiosk and a Program, and whoever may not edit
	// one (ADR 0041, 0046).
	handle(guest, "POST /api/dashboards", func(w http.ResponseWriter, r *http.Request) {
		var d dashboard.Dashboard
		if !decode(w, r, &d) {
			return
		}
		by, _ := identity(r)
		id, err := dash.Create(by, d)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	})
	handle(guest, "PUT /api/dashboards/{id}", func(w http.ResponseWriter, r *http.Request) {
		var d dashboard.Dashboard
		if decode(w, r, &d) {
			by, _ := identity(r)
			reply(w, dash.Save(by, r.PathValue("id"), d))
		}
	})
	handle(guest, "DELETE /api/dashboards/{id}", func(w http.ResponseWriter, r *http.Request) {
		by, _ := identity(r)
		reply(w, dash.Delete(by, r.PathValue("id")))
	})
	// A Person's list of the Dashboards they see: their order, the hidden ones
	// (ADR 0044).
	handle(guest, "PUT /api/me/dashboards", func(w http.ResponseWriter, r *http.Request) {
		var list []dashboard.Entry
		if decode(w, r, &list) {
			by, _ := identity(r)
			reply(w, dash.SaveList(by, list))
		}
	})
	// A camera's Picture shows the home as it is now (ADR 0036).
	handle(guest, "GET /api/picture", picture(cams))
	handle(guest, "POST /api/live-view", liveView(cams))
	// Runs an Automation from its Manual trigger step, at once: how the Run ended.
	handle(guest, "POST /api/automations/{id}/steps/{step}/run", func(w http.ResponseWriter, r *http.Request) {
		end, err := automations.Trigger(r.PathValue("id"), r.PathValue("step"), origin(r))
		by, _ := identity(r)
		respond(w, runEndFor(by.Level, end), err)
	})

	// A Member also reads the home's past and how its Automations are built.
	handle(member, "GET /api/automations", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, automations.Documents())
	})
	handle(member, "GET /api/automations/{id}/state", func(w http.ResponseWriter, r *http.Request) {
		state, err := automations.State(r.PathValue("id"))
		respond(w, state, err)
	})
	handle(member, "GET /api/automations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := hist.Runs(r.PathValue("id"))
		respond(w, runs, err)
	})
	handle(member, "GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		trace, err := hist.Trace(r.PathValue("id"))
		respond(w, trace, err)
	})
	// A Target's latest Commands: ?target=<key>.
	handle(member, "GET /api/commands", func(w http.ResponseWriter, r *http.Request) {
		t, err := home.ParseTarget(r.URL.Query().Get("target"))
		if err != nil {
			reply(w, err)
			return
		}
		commands, err := hist.Commands(t)
		respond(w, commands, err)
	})
	handle(member, "POST /api/history", readHistory(h, hist))
	handle(member, "POST /api/history/periods", readPeriods(h, hist))
	// A camera's Recordings, kept by its own system (ADR 0038).
	handle(member, "GET /api/recordings", recordings(cams))
	handle(member, "GET /api/recordings/video", recordingMedia(cams, bridge.Video))
	handle(member, "GET /api/recordings/thumbnail", recordingMedia(cams, bridge.Thumbnail))
	handle(member, "GET /api/lost-entries", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]uint64{"lost": hist.Lost()})
	})

	// An Admin also edits the home and sees what Oiko is built from.
	handle(admin, "GET /api/build", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			build.Build
			Install string            `json:"install"`
			Bridges map[string]string `json:"bridges"`
		}{b, install, bridges})
	})
	handle(admin, "PATCH /api/devices/{id}", rename(h))
	handle(admin, "PUT /api/icon", setIcon(h))
	handle(admin, "PUT /api/area", setArea(h))
	handle(admin, "POST /api/areas", defineArea(h))
	handle(admin, "PUT /api/areas/{id}", defineArea(h))
	// Its Area Aggregates go with it, and their History: Home announces it.
	handle(admin, "DELETE /api/areas/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.DeleteArea(home.AreaID(r.PathValue("id"))))
	})
	// How the dashboard shows Area {id}: the tiles it hides, the kinds of Area
	// Aggregates its header leaves out.
	handle(admin, "PUT /api/areas/{id}/display", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hidden           []home.Target `json:"hidden"`
			HiddenAggregates []string      `json:"hiddenAggregates"`
		}
		if decode(w, r, &req) {
			reply(w, h.SetAreaDisplay(home.AreaID(r.PathValue("id")), req.Hidden, req.HiddenAggregates))
		}
	})
	// Area {id}'s grid on the dashboard: its columns and where its tiles sit.
	handle(admin, "PUT /api/areas/{id}/layout", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Columns int              `json:"columns"`
			Layout  []home.Placement `json:"layout"`
		}
		if decode(w, r, &req) {
			reply(w, h.SetAreaLayout(home.AreaID(r.PathValue("id")), req.Columns, req.Layout))
		}
	})
	handle(admin, "PUT /api/areas", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Order []home.AreaID `json:"order"`
		}
		if decode(w, r, &req) {
			reply(w, h.OrderAreas(req.Order))
		}
	})
	handle(admin, "DELETE /api/devices/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.Delete(home.DeviceID(r.PathValue("id"))))
	})
	handle(admin, "POST /api/devices/{id}/replace", replace(h))
	handle(admin, "POST /api/aggregates", defineAggregate(h))
	handle(admin, "PUT /api/aggregates/{id}", defineAggregate(h))
	handle(admin, "DELETE /api/aggregates/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.DeleteAggregate(home.AggregateID(r.PathValue("id"))))
	})
	handle(admin, "POST /api/flags", defineFlag(h))
	handle(admin, "PUT /api/flags/{id}", defineFlag(h))
	handle(admin, "DELETE /api/flags/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, h.DeleteFlag(home.FlagID(r.PathValue("id"))))
	})
	handle(admin, "POST /api/automations", defineAutomation(automations))
	handle(admin, "PUT /api/automations/{id}", defineAutomation(automations))
	handle(admin, "DELETE /api/automations/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, automations.Delete(r.PathValue("id")))
	})
	handle(admin, "DELETE /api/automations/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		reply(w, hist.ClearRuns(r.PathValue("id")))
	})
	handle(admin, "DELETE /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, hist.DeleteRun(r.PathValue("id")))
	})

	// Access rules its own endpoints: anyone signs in and manages their own
	// credentials; only an Admin Person manages access (ADR 0023).
	handleAccess(mux, acc, public)
	handlePasskeys(mux, acc, public)
	handlePrograms(mux, acc)
	handleKiosks(mux, acc, public)
	handlePersons(mux, acc, dash, public)
	handleSessions(mux, acc)
	handleAudit(mux, acc, hist)

	client := files(static)
	mux.Handle("GET /", client)
	// The pages a Setup link, a Sign-in link and a Kiosk's pairing QR code
	// open, their secret in the fragment, which never reaches Oiko.
	page := func(w http.ResponseWriter, r *http.Request) {
		r = r.Clone(r.Context())
		r.URL.Path = "/"
		client.ServeHTTP(w, r)
	}
	mux.HandleFunc("GET /setup", page)
	mux.HandleFunc("GET /sign-in", page)
	mux.HandleFunc("GET /pair", page)
	return secure(http.NewCrossOriginProtection().Handler(identify(acc, public, mux)))
}

// Bounds on the HTTP server and the event stream (ADR 0034); variables so
// tests can shorten them.
var (
	readTimeout      = 30 * time.Second
	streamWriteLimit = 30 * time.Second // each write to the event stream
	keepaliveEvery   = 15 * time.Second
)

// Server serves handler on addr within the bounds of ADR 0034. It has no
// WriteTimeout: the event stream bounds each of its writes instead.
func Server(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       readTimeout,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}

// headers are the ones every response carries (ADR 0034).
var headers = map[string]string{
	"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; media-src 'self' blob:; font-src 'self'; " +
		"connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'",
	"X-Content-Type-Options":       "nosniff",
	"Referrer-Policy":              "no-referrer",
	"Cross-Origin-Opener-Policy":   "same-origin",
	"Cross-Origin-Resource-Policy": "same-origin",
	// Every feature denied but sharing and copying a Sign-in link (ADR 0030)
	// and autoplay, which starts a Live view once its camera woke, seconds
	// after the tap that asked for it (ADR 0037).
	// Passkeys keep their default, Oiko itself: sign-in needs them.
	"Permissions-Policy": "accelerometer=(), autoplay=(self), camera=(), clipboard-read=(), display-capture=(), " +
		"encrypted-media=(), fullscreen=(), geolocation=(), gyroscope=(), hid=(), idle-detection=(), magnetometer=(), " +
		"microphone=(), midi=(), payment=(), picture-in-picture=(), screen-wake-lock=(), serial=(), usb=(), " +
		"xr-spatial-tracking=(), web-share=(self), clipboard-write=(self)",
	"Strict-Transport-Security": "max-age=31536000",
}

// secure adds headers to every response, and keeps every API response out of
// caches.
func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// files serves the web client in static. What is under assets/ is named by
// its content hash and cached for good; anything else is revalidated against
// an ETag of its content, computed here since embedded files have no
// modification time.
func files(static fs.FS) http.Handler {
	etags := map[string]string{}
	err := fs.WalkDir(static, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(static, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		etags[name] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	if err != nil {
		panic(err) // embedded: a broken build
	}
	serve := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			if name == "" || strings.HasSuffix(name, "/") {
				name += "index.html"
			}
			w.Header().Set("Cache-Control", "no-cache")
			if etag, ok := etags[name]; ok {
				w.Header().Set("ETag", etag)
			}
		}
		serve.ServeHTTP(w, r)
	})
}

// respond answers v in JSON, or the status matching err.
func respond(w http.ResponseWriter, v any, err error) {
	if err != nil {
		reply(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// updates streams a snapshot, then every Update, as Server-Sent Events, as
// the identity's Access level lets it see them: a Guest sees only what it can
// press, never who did what (ADR 0031). The Status of each module's Releases
// comes to an Admin with the snapshot, and again, as a "releases" message,
// each time it changes (ADR 0023). The Dashboards a Person sees come with the
// snapshot too, as they see them, with their list, and again, whole, as a
// "dashboards" message, each time what they see of them changes (ADR 0041,
// 0044, 0046): one of them edited, their list saved, or for a Guest, the
// Automations it may press. Updates available at
// once are written together and flushed once. A client that stops reading
// ends the stream, at the first write that does not go through within
// streamWriteLimit. It counts as a use of its Session or Token at each
// keepalive, and ends at once when either ends, or the identity's access
// changes: the client reconnects and gets what its level sees now.
func updates(h *home.Home, acc *access.Store, dash *dashboard.Store, releases *release.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		snap, ch, cancel := h.Subscribe()
		automations := snap.Automations // all of them: what a Dashboard is filtered by
		snap = snapshotFor(id.Level, snap)
		defer cancel()
		statuses, changed := []release.Status{}, (<-chan struct{})(nil) // nil: never
		if id.Level.Allows(access.Admin) {
			statuses, changed = releases.Statuses()
		}
		dashboards, rearranged := dash.Dashboards(id)
		list := dash.List(id) // after the channel: a list saved since closes it
		shown, listed := dashboardsFor(id, dashboards, automations), list
		w.Header().Set("Content-Type", "text/event-stream")
		rc := http.NewResponseController(w)
		defer rc.SetWriteDeadline(time.Time{}) // the connection may serve other requests
		send := func(write func() bool) bool {
			return rc.SetWriteDeadline(time.Now().Add(streamWriteLimit)) == nil && write() && rc.Flush() == nil
		}
		event := func(v any) func() bool {
			return func() bool { return writeEvent(w, v) == nil }
		}

		if !send(event(struct {
			Kind string `json:"kind"`
			home.Snapshot
			Releases   []release.Status      `json:"releases"`
			Dashboards []dashboard.Dashboard `json:"dashboards,omitzero"` // none for a Kiosk or a Program, [] for a Person without any
			List       []dashboard.Entry     `json:"list,omitzero"`       // a Person's
		}{"snapshot", snap, statuses, shown, listed})) {
			return
		}
		// writeDashboards writes the Dashboards as the identity sees them, and its
		// list, if that changed.
		writeDashboards := func() bool {
			now := dashboardsFor(id, dashboards, automations)
			if reflect.DeepEqual(now, shown) && slices.Equal(list, listed) {
				return true
			}
			shown, listed = now, list
			return writeEvent(w, struct {
				Kind       string                `json:"kind"`
				Dashboards []dashboard.Dashboard `json:"dashboards"`
				List       []dashboard.Entry     `json:"list"`
			}{"dashboards", shown, listed}) == nil
		}
		keepalive := time.NewTicker(keepaliveEvery)
		defer keepalive.Stop()
		for {
			select {
			case u, ok := <-ch:
				// Closed: this observer fell too far behind. Ending the stream makes the
				// browser reconnect and start over from a fresh snapshot.
				if !ok || !send(func() bool {
					refilter := false
					return writeBatch(w, u, ch, id.Level, func(u home.Update) {
						// What a Guest sees of a Dashboard follows the Automations it may press.
						if u.Kind == home.AutomationsChanged && !id.Level.Allows(access.Member) {
							automations, refilter = u.Automations, true
						}
					}) && (!refilter || writeDashboards())
				}) {
					return
				}
			case <-changed:
				statuses, changed = releases.Statuses()
				if !send(event(struct {
					Kind     string           `json:"kind"`
					Releases []release.Status `json:"releases"`
				}{"releases", statuses})) {
					return
				}
			case <-rearranged:
				dashboards, rearranged = dash.Dashboards(id)
				list = dash.List(id)
				if !send(writeDashboards) {
					return
				}
			case <-keepalive.C:
				if _, ok := resolve(acc, r); !ok {
					return
				}
				if !send(func() bool { _, err := w.Write([]byte(": keepalive\n\n")); return err == nil }) {
					return
				}
			case <-id.Ended:
				return
			case <-r.Context().Done():
				return
			}
		}
	}
}

// writeBatch writes u and every Update already queued behind it, each handed
// to observe first, then as level l sees it, if it does; it reports false if
// the stream must end.
func writeBatch(w http.ResponseWriter, u home.Update, ch <-chan home.Update, l access.Level, observe func(home.Update)) bool {
	for {
		observe(u)
		if v, ok := updateFor(l, u); ok && writeEvent(w, v) != nil {
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

// command addresses a Target by its key. Setting a configuration Capability
// is editing the home: an Admin's (ADR 0023).
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
		if by, _ := identity(r); !by.Level.Allows(access.Admin) {
			for k := range req.Values {
				if c, err := h.Capability(req.Target.Ref(k)); err == nil && c.Category == home.Config {
					http.Error(w, fmt.Sprintf("%v: %s is a configuration Capability: only an Admin changes it", access.ErrRefused, c.Label), http.StatusForbidden)
					return
				}
			}
		}
		id, err := h.Command(req.Target, home.Request{Values: req.Values, Transition: time.Duration(req.Transition * float64(time.Second)), Origin: origin(r)})
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

// defineArea creates an Area, or sets the Name and Icon of Area {id} when
// there is one.
func defineArea(h *home.Home) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
			Icon string `json:"icon"`
		}
		if !decode(w, r, &req) {
			return
		}
		if id := r.PathValue("id"); id != "" {
			reply(w, h.EditArea(home.AreaID(id), req.Name, req.Icon))
			return
		}
		_, err := h.CreateArea(req.Name, req.Icon)
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

// reply answers 204 on success, or the status matching a Home, camera or
// access error.
func reply(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, home.ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, home.ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, home.ErrBridgeOffline), errors.Is(err, camera.ErrBusy):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, camera.ErrNoAnswer):
		http.Error(w, err.Error(), http.StatusBadGateway)
	case errors.Is(err, home.ErrNotRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, access.ErrRefused):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
