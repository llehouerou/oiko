package api

import (
	"net/http"
	"net/url"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// pairingCookie holds a pairing request's claim secret, on the screen that
// made it alone (ADR 0029).
const pairingCookie = "__Host-oiko-pairing"

// kioskCookieAge is how long a Kiosk's Session cookie lasts, set again on
// each request: the longest browsers keep one. Oiko decides when the Session
// ends (ADR 0025).
const kioskCookieAge = 400 * 24 * time.Hour

// handleKiosks serves Kiosk pairing, from the screen and to the approving
// Admin, and the Kiosks to an Admin Person; access refuses anyone else.
func handleKiosks(mux *http.ServeMux, acc *access.Store, public *url.URL) {
	by := func(r *http.Request) access.Identity {
		id, _ := identity(r)
		return id
	}
	// A pairing request from a screen: its approval secret, for the QR code,
	// and its claim secret, in a cookie that lasts as long as the browser.
	mux.HandleFunc("POST /api/kiosk-pairing", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		approval, claim := acc.RequestPairing(access.Browser(r.UserAgent()))
		setCookie(w, pairingCookie, claim, 0)
		writeJSON(w, http.StatusCreated, map[string]string{"secret": approval})
	}))
	// Polled by the screen: 202 while no Admin approved, then the Kiosk's
	// Session in its cookie; refused once the request expired, when the
	// screen makes a new one.
	mux.HandleFunc("POST /api/kiosk-pairing/claim", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		var claim string
		if c, err := r.Cookie(pairingCookie); err == nil {
			claim = c.Value
		}
		secret, err := acc.ClaimPairing(claim)
		switch {
		case err != nil:
			setCookie(w, pairingCookie, "", -1)
			reply(w, err)
		case secret == "":
			w.WriteHeader(http.StatusAccepted)
		default:
			setCookie(w, pairingCookie, "", -1)
			setCookie(w, sessionCookie, secret, int(kioskCookieAge.Seconds()))
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	type approval struct {
		Secret string       `json:"secret"`
		Kiosk  string       `json:"kiosk"` // an existing Kiosk's id, or "" for a new one
		Name   string       `json:"name"`
		Level  access.Level `json:"level"`
	}
	// The pairing request of an approval secret, as the Admin about to
	// approve it sees it.
	mux.HandleFunc("POST /api/kiosk-pairing/request", func(w http.ResponseWriter, r *http.Request) {
		var req approval
		if !decode(w, r, &req) {
			return
		}
		p, err := acc.PairingRequest(by(r), req.Secret)
		respond(w, struct {
			Created time.Time `json:"created"`
			Browser string    `json:"browser"`
		}{p.Created, p.Browser}, err)
	})
	mux.HandleFunc("POST /api/kiosk-pairing/approve", func(w http.ResponseWriter, r *http.Request) {
		var req approval
		if !decode(w, r, &req) {
			return
		}
		id, err := acc.ApprovePairing(by(r), req.Secret, req.Kiosk, req.Name, req.Level)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	})

	type person struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"` // none once they are removed
	}
	type kiosk struct {
		ID       string       `json:"id"`
		Name     string       `json:"name"`
		Level    access.Level `json:"level"`
		PairedBy person       `json:"pairedBy"`
		Paired   time.Time    `json:"paired"`
		LastUse  time.Time    `json:"lastUse,omitzero"`
		SignedIn bool         `json:"signedIn"`
	}
	mux.HandleFunc("GET /api/kiosks", func(w http.ResponseWriter, r *http.Request) {
		ks, err := acc.Kiosks(by(r))
		if err != nil {
			reply(w, err)
			return
		}
		list := make([]kiosk, len(ks))
		for i, k := range ks {
			list[i] = kiosk{k.ID, k.Name, k.Level, person{k.Pairer, acc.PersonName(k.Pairer)}, k.Paired, k.LastUse, k.SignedIn}
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("PUT /api/kiosks/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req approval
		if decode(w, r, &req) {
			reply(w, acc.EditKiosk(by(r), r.PathValue("id"), req.Name, req.Level))
		}
	})
	mux.HandleFunc("DELETE /api/kiosks/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, acc.RemoveKiosk(by(r), r.PathValue("id")))
	})
	mux.HandleFunc("DELETE /api/kiosks/{id}/session", func(w http.ResponseWriter, r *http.Request) {
		reply(w, acc.SignOutKiosk(by(r), r.PathValue("id")))
	})
}
