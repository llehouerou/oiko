package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// handlePasskeys serves signing in with a Passkey, confirming one for
// step-up, and a Person's own Passkeys (ADR 0024, 0025). Each ceremony runs
// at the origin of the page asking, one where sign-in works: options first,
// for the browser's passkey sheet, then its answer.
func handlePasskeys(mux *http.ServeMux, acc *access.Store, public *url.URL) {
	origin := func(r *http.Request) string { return r.Header.Get("Origin") }
	// answer is the browser's PublicKeyCredential in r, as JSON; false once
	// refused.
	answer := func(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
		var a json.RawMessage
		return a, decode(w, r, &a)
	}

	// Username-less: the passkey sheet picks the account, any Passkey of this
	// Oiko; a phone's, across devices, needs nothing more.
	mux.HandleFunc("POST /api/sign-in/passkey/options", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		options, err := acc.BeginSignIn(origin(r))
		respond(w, options, err)
	}))
	mux.HandleFunc("POST /api/sign-in/passkey", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		a, ok := answer(w, r)
		if !ok {
			return
		}
		secret, err := acc.FinishSignIn(origin(r), a, access.Browser(r.UserAgent()))
		if err != nil {
			reply(w, err)
			return
		}
		setCookie(w, sessionCookie, secret, int(access.SessionLimit.Seconds()))
		w.WriteHeader(http.StatusNoContent)
	}))

	// Step-up: confirming one of their Passkeys makes this Session fresh.
	mux.HandleFunc("POST /api/step-up/options", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		options, err := acc.BeginStepUp(sessionSecret(r), origin(r))
		respond(w, options, err)
	}))
	mux.HandleFunc("POST /api/step-up", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		if a, ok := answer(w, r); ok {
			reply(w, acc.FinishStepUp(sessionSecret(r), origin(r), a))
		}
	}))

	// The signed-in Person's own Passkeys, and, to a fresh Admin Person,
	// anyone's (ADR 0025).
	type passkey struct {
		ID       string    `json:"id"`
		Provider string    `json:"provider,omitempty"` // none when unknown
		Created  time.Time `json:"created"`
		LastUse  time.Time `json:"lastUse,omitzero"`
	}
	list := func(w http.ResponseWriter, r *http.Request, person string) {
		id, _ := identity(r)
		passkeys, err := acc.Passkeys(id, person)
		if err != nil {
			reply(w, err)
			return
		}
		list := make([]passkey, len(passkeys))
		for i, p := range passkeys {
			list[i] = passkey{p.ID.String(), access.Provider(p.AAGUID), p.Created, p.LastUse}
		}
		writeJSON(w, http.StatusOK, list)
	}
	mux.HandleFunc("GET /api/me/passkeys", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		list(w, r, id.ID)
	})
	mux.HandleFunc("GET /api/persons/{id}/passkeys", func(w http.ResponseWriter, r *http.Request) {
		list(w, r, r.PathValue("id"))
	})
	mux.HandleFunc("DELETE /api/persons/{id}/passkeys/{passkey}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		reply(w, acc.RemovePasskey(id, r.PathValue("id"), r.PathValue("passkey")))
	})
	mux.HandleFunc("POST /api/me/passkeys/options", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		options, err := acc.BeginPasskey(id, origin(r))
		respond(w, options, err)
	}))
	mux.HandleFunc("POST /api/me/passkeys", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		a, ok := answer(w, r)
		if !ok {
			return
		}
		id, _ := identity(r)
		added, err := acc.FinishPasskey(id, origin(r), a)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": added})
	}))
	mux.HandleFunc("DELETE /api/me/passkeys/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		reply(w, acc.RemovePasskey(id, id.ID, r.PathValue("id")))
	})
}
