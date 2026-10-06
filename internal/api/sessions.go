package api

import (
	"net/http"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// handleSessions serves the signed-in Person's own Sessions, which they end
// without step-up, and, to a fresh Admin Person, anyone's (ADR 0025). An
// ended Session's event stream closes at once.
func handleSessions(mux *http.ServeMux, acc *access.Store) {
	type named struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"` // none once they are removed
	}
	type session struct {
		ID       string    `json:"id"`
		Browser  string    `json:"browser"`
		Method   string    `json:"method"`             // "setup", "passkey" or "link"
		Provider string    `json:"provider,omitempty"` // of the Passkey it signed in with, if known
		By       *named    `json:"by,omitempty"`       // who created the link it signed in with, when not its Person
		SignedIn time.Time `json:"signedIn"`
		LastUse  time.Time `json:"lastUse"`
		Current  bool      `json:"current"` // the one asking: this device
	}
	list := func(w http.ResponseWriter, r *http.Request, person string) {
		id, _ := identity(r)
		sessions, err := acc.Sessions(id, person)
		if err != nil {
			reply(w, err)
			return
		}
		list := make([]session, len(sessions))
		for i, x := range sessions {
			list[i] = session{x.ID, x.Browser, x.Method, x.Provider, nil, x.SignedIn, x.LastUse, x.ID == id.Session}
			if x.By != "" {
				list[i].By = &named{x.By, acc.PersonName(x.By)}
			}
		}
		writeJSON(w, http.StatusOK, list)
	}
	mux.HandleFunc("GET /api/me/sessions", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		list(w, r, id.ID)
	})
	// Signs out every other device.
	mux.HandleFunc("DELETE /api/me/sessions", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		reply(w, acc.EndOtherSessions(id))
	})
	mux.HandleFunc("DELETE /api/me/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		reply(w, acc.EndSession(id, id.ID, r.PathValue("id")))
	})
	mux.HandleFunc("GET /api/persons/{id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		list(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("DELETE /api/persons/{id}/sessions/{session}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		reply(w, acc.EndSession(id, r.PathValue("id"), r.PathValue("session")))
	})
}
