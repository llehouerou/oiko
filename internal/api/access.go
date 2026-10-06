package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/home"
)

// sessionCookie holds a Session's secret; Oiko, not its expiry, decides when
// the Session ends (ADR 0025).
const sessionCookie = "__Host-oiko-session"

type identityKey struct{}

// openEndpoints are those of the API a request without credentials
// reaches: who am I, the Setup link, signing in with a Passkey or a Sign-in
// link, pairing a Kiosk from its screen, and signing out, which clears the
// cookie of a Session that already ended.
var openEndpoints = map[string]bool{
	"/api/me": true, "/api/setup": true, "/api/sign-in/passkey/options": true, "/api/sign-in/passkey": true,
	"/api/sign-in/link/person": true, "/api/sign-in/link": true, "/api/sign-out": true,
	"/api/kiosk-pairing": true, "/api/kiosk-pairing/claim": true,
}

// identify resolves each request, by its Token or else its Session, to its
// identity, which the request then carries. Without either, it reaches the
// web client and the open endpoints only; with a Token that is not valid,
// nothing. The network never stands for credentials (ADR 0027). A Kiosk's
// cookie is set again, for a Session with no limit but idleness (ADR 0029).
func identify(acc *access.Store, public *url.URL, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := resolve(acc, r)
		_, isBearer := bearer(r)
		switch {
		case ok:
			if id.Kind == access.KioskKind {
				setCookie(w, sessionCookie, sessionSecret(r), int(kioskCookieAge.Seconds()))
			}
			r = r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
		case isBearer:
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "this Token is not valid: it was revoked, replaced or never existed", http.StatusUnauthorized)
			return
		case strings.HasPrefix(r.URL.Path, "/api/") && !openEndpoints[r.URL.Path]:
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, refusal(public, r), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// refusal says why r, without credentials, is refused: without a Public URL,
// only http://localhost offers sign-in (ADR 0027). Its address decides
// nothing but these words.
func refusal(public *url.URL, r *http.Request) string {
	const token = "a Program sends its Token as Authorization: Bearer"
	if public == nil && (&url.URL{Host: r.Host}).Hostname() != "localhost" {
		return "a Public URL must be configured: until then, only http://localhost offers sign-in; " + token
	}
	return "sign in first; " + token
}

// identity is who r is; false when it is anonymous.
func identity(r *http.Request) (access.Identity, bool) {
	id, ok := r.Context().Value(identityKey{}).(access.Identity)
	return id, ok
}

// needs serves next only to an identity whose Access level allows level
// (ADR 0023).
func needs(level access.Level, next http.HandlerFunc) http.HandlerFunc {
	who := map[access.Level]string{access.Guest: "anyone signed in", access.Member: "a Member or an Admin", access.Admin: "an Admin"}[level]
	return func(w http.ResponseWriter, r *http.Request) {
		if id, _ := identity(r); !id.Level.Allows(level) {
			http.Error(w, fmt.Sprintf("%v: only %s may do this", access.ErrRefused, who), http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// origin is the Origin of a Command or a Manual trigger r asks for (ADR
// 0031): r's identity.
func origin(r *http.Request) home.Origin {
	id, _ := identity(r)
	switch id.Kind {
	case access.PersonKind:
		return home.Origin{Person: id.ID}
	case access.KioskKind:
		return home.Origin{Kiosk: id.ID}
	case access.ProgramKind:
		return home.Origin{Program: id.ID}
	}
	return home.Origin{}
}

// resolve answers who holds r's Token or else its Session, counting it as a
// use; false without one that lasts.
func resolve(acc *access.Store, r *http.Request) (access.Identity, bool) {
	if token, ok := bearer(r); ok {
		return acc.ResolveToken(token)
	}
	return acc.Resolve(sessionSecret(r))
}

// sessionSecret is the secret of r's Session cookie; "" if it has none.
func sessionSecret(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// bearer is r's Token, accepted only in its Authorization header, never in
// its URL (ADR 0028); false if it sends none. A malformed one is "", never
// valid.
func bearer(r *http.Request) (string, bool) {
	f := strings.Fields(r.Header.Get("Authorization"))
	if len(f) == 0 || !strings.EqualFold(f[0], "Bearer") {
		return "", false
	}
	if len(f) != 2 {
		return "", true
	}
	return f[1], true
}

// handleAccess serves who a request is, the Setup link and signing out.
func handleAccess(mux *http.ServeMux, acc *access.Store, public *url.URL) {
	var publicURL *string
	if public != nil {
		s := public.String()
		publicURL = &s
	}
	// Who am I: the identity, if signed in, and what the web client needs
	// before: whether Oiko has an Admin, where sign-in works.
	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		me := struct {
			Identity  map[access.Kind]string `json:"identity"` // {"person": id}, {"kiosk": id} or {"program": id}; null if anonymous
			Name      string                 `json:"name,omitempty"`
			Level     access.Level           `json:"level,omitempty"`
			Fresh     bool                   `json:"fresh"`
			Claimed   bool                   `json:"claimed"`
			PublicURL *string                `json:"publicUrl"`
		}{Claimed: acc.Claimed(), PublicURL: publicURL}
		if id, ok := identity(r); ok {
			me.Identity, me.Name, me.Level, me.Fresh = map[access.Kind]string{id.Kind: id.ID}, id.Name, id.Level, id.Fresh
		}
		writeJSON(w, http.StatusOK, me)
	})
	// Claims a fresh Oiko: the Setup link's secret and the first Admin's Name.
	mux.HandleFunc("POST /api/setup", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Secret string `json:"secret"`
			Name   string `json:"name"`
		}
		if !decode(w, r, &req) {
			return
		}
		secret, err := acc.Claim(req.Secret, req.Name, access.Browser(r.UserAgent()))
		if err != nil {
			reply(w, err)
			return
		}
		setCookie(w, sessionCookie, secret, int(access.SessionLimit.Seconds()))
		w.WriteHeader(http.StatusNoContent)
	}))
	// The Names of the Persons, Kiosks and Programs, by kind then id, for
	// showing Origins: a Member's, an Admin's.
	mux.HandleFunc("GET /api/names", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		names, err := acc.Names(id)
		respond(w, names, err)
	})
	mux.HandleFunc("POST /api/sign-out", func(w http.ResponseWriter, r *http.Request) {
		if err := acc.SignOut(sessionSecret(r)); err != nil {
			reply(w, err)
			return
		}
		setCookie(w, sessionCookie, "", -1)
		w.WriteHeader(http.StatusNoContent)
	})
}

// setCookie sets cookie name, one of Oiko's secrets, to value for maxAge
// seconds: 0 for as long as the browser runs, a negative maxAge deletes it.
func setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

// atSignInOrigin serves next only to a page of the Public URL or of
// http://localhost, the only origins where a browser signs in (ADR 0027),
// and never to a Kiosk, on which no one signs in (ADR 0029).
func atSignInOrigin(public *url.URL, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		u, err := url.Parse(origin)
		local := err == nil && u.Scheme == "http" && u.Hostname() == "localhost"
		if !local && (public == nil || origin != public.String()) {
			where := "http://localhost, until a Public URL is configured"
			if public != nil {
				where = public.String()
			}
			http.Error(w, fmt.Sprintf("sign in at %s", where), http.StatusForbidden)
			return
		}
		if id, _ := identity(r); id.Kind == access.KioskKind {
			http.Error(w, fmt.Sprintf("%v: this screen is a Kiosk; an Admin signs it out first", access.ErrRefused), http.StatusForbidden)
			return
		}
		next(w, r)
	}
}
