package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/llehouerou/oiko/internal/access"
)

// sessionCookie holds a Session's secret; Oiko, not its expiry, decides when
// the Session ends (ADR 0025).
const sessionCookie = "__Host-oiko-session"

type identityKey struct{}

// identify resolves the Session of each request, if any, to its identity,
// which the request then carries. A request without one goes on anonymous.
func identify(acc *access.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := resolve(acc, r); ok {
			r = r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
		}
		next.ServeHTTP(w, r)
	})
}

// identity is who r is; false when it is anonymous.
func identity(r *http.Request) (access.Identity, bool) {
	id, ok := r.Context().Value(identityKey{}).(access.Identity)
	return id, ok
}

// resolve answers who holds r's Session, counting it as a use; false without
// one that lasts.
func resolve(acc *access.Store, r *http.Request) (access.Identity, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return access.Identity{}, false
	}
	return acc.Resolve(c.Value)
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
		type person struct {
			Person string `json:"person"`
		}
		me := struct {
			Identity  *person      `json:"identity"`
			Name      string       `json:"name,omitempty"`
			Level     access.Level `json:"level,omitempty"`
			Fresh     bool         `json:"fresh"`
			Claimed   bool         `json:"claimed"`
			PublicURL *string      `json:"publicUrl"`
		}{Claimed: acc.Claimed(), PublicURL: publicURL}
		if id, ok := identity(r); ok {
			me.Identity, me.Name, me.Level, me.Fresh = &person{id.Person.ID}, id.Person.Name, id.Person.Level, id.Fresh
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
		setSession(w, secret, int(access.SessionLimit.Seconds()))
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/sign-out", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			if err := acc.SignOut(c.Value); err != nil {
				reply(w, err)
				return
			}
		}
		setSession(w, "", -1)
		w.WriteHeader(http.StatusNoContent)
	})
}

// setSession sets the Session cookie to secret for maxAge seconds; a
// negative maxAge deletes it.
func setSession(w http.ResponseWriter, secret string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: secret, Path: "/", MaxAge: maxAge,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

// atSignInOrigin serves next only to a page of the Public URL or of
// http://localhost, the only origins where a browser signs in (ADR 0027).
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
		next(w, r)
	}
}
