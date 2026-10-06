package api

import (
	"net/http"
	"net/url"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// handlePersons serves the Persons to an Admin Person, their Sign-in links,
// a Person's own Name, and signing in by link (ADR 0026, 0030). A link's
// secret travels in request bodies only, never in a URL: its page has it in
// its fragment.
func handlePersons(mux *http.ServeMux, acc *access.Store, public *url.URL) {
	type named struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"` // none once they are removed
	}
	type link struct {
		Creator named     `json:"creator"`
		Created time.Time `json:"created"`
		Expires time.Time `json:"expires"`
	}
	type person struct {
		ID    string       `json:"id"`
		Name  string       `json:"name"`
		Level access.Level `json:"level"`
		Ends  time.Time    `json:"ends,omitzero"` // a Guest's end date, if any
		Link  *link        `json:"link"`          // their pending Sign-in link; null if none
	}
	// What a Person is to be: their Name, Access level and, a Guest's only,
	// end date: null or absent for none.
	type definition struct {
		Name  string       `json:"name"`
		Level access.Level `json:"level"`
		Ends  time.Time    `json:"ends"`
	}
	type secret struct {
		Secret string `json:"secret"`
	}
	by := func(r *http.Request) access.Identity {
		id, _ := identity(r)
		return id
	}

	mux.HandleFunc("GET /api/persons", func(w http.ResponseWriter, r *http.Request) {
		ps, err := acc.Persons(by(r))
		if err != nil {
			reply(w, err)
			return
		}
		list := make([]person, len(ps))
		for i, p := range ps {
			list[i] = person{p.ID, p.Name, p.Level, p.Ends, nil}
			if l := p.Link; l != nil {
				list[i].Link = &link{named{l.Creator, acc.PersonName(l.Creator)}, l.Created, l.Expires}
			}
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("POST /api/persons", func(w http.ResponseWriter, r *http.Request) {
		var req definition
		if !decode(w, r, &req) {
			return
		}
		p, err := acc.CreatePerson(by(r), req.Name, req.Level, req.Ends)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": p.ID})
	})
	mux.HandleFunc("PUT /api/persons/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req definition
		if decode(w, r, &req) {
			reply(w, acc.EditPerson(by(r), r.PathValue("id"), req.Name, req.Level, req.Ends))
		}
	})
	mux.HandleFunc("DELETE /api/persons/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, acc.RemovePerson(by(r), r.PathValue("id")))
	})
	// A new Sign-in link for Person {id}, an Admin's for anyone, or anyone's
	// for themself: its secret, shown this once, and when it expires.
	mux.HandleFunc("POST /api/persons/{id}/sign-in-link", func(w http.ResponseWriter, r *http.Request) {
		s, expires, err := acc.CreateLink(by(r), r.PathValue("id"))
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, struct {
			Secret  string    `json:"secret"`
			Expires time.Time `json:"expires"`
		}{s, expires})
	})
	mux.HandleFunc("DELETE /api/persons/{id}/sign-in-link", func(w http.ResponseWriter, r *http.Request) {
		reply(w, acc.RevokeLink(by(r), r.PathValue("id")))
	})

	// The signed-in Person's own Name.
	mux.HandleFunc("PUT /api/me", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if decode(w, r, &req) {
			reply(w, acc.Rename(by(r), req.Name))
		}
	})

	// Whom a Sign-in link signs in, for its welcome page, before it is spent:
	// their Name, and a Guest's end date, if any.
	mux.HandleFunc("POST /api/sign-in/link/person", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		var req secret
		if decode(w, r, &req) {
			p, err := acc.LinkedPerson(req.Secret, access.Browser(r.UserAgent()))
			respond(w, struct {
				Name string    `json:"name"`
				Ends time.Time `json:"ends,omitzero"`
			}{p.Name, p.Ends}, err)
		}
	}))
	mux.HandleFunc("POST /api/sign-in/link", atSignInOrigin(public, func(w http.ResponseWriter, r *http.Request) {
		var req secret
		if !decode(w, r, &req) {
			return
		}
		session, err := acc.SignInWithLink(req.Secret, access.Browser(r.UserAgent()))
		if err != nil {
			reply(w, err)
			return
		}
		setSession(w, session, int(access.SessionLimit.Seconds()))
		w.WriteHeader(http.StatusNoContent)
	}))
}
