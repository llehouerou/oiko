package api

import (
	"net/http"
	"time"

	"github.com/llehouerou/oiko/internal/access"
)

// handlePrograms serves the Programs and their Tokens to an Admin Person;
// access refuses anyone else, a Program first (ADR 0028).
func handlePrograms(mux *http.ServeMux, acc *access.Store) {
	type token struct {
		Generated time.Time `json:"generated"`
		LastUse   time.Time `json:"lastUse,omitzero"`
	}
	type creator struct {
		ID   string `json:"id"`
		Name string `json:"name,omitempty"` // none once they are removed
	}
	type program struct {
		ID      string       `json:"id"`
		Name    string       `json:"name"`
		Level   access.Level `json:"level"`
		Creator creator      `json:"creator"`
		Created time.Time    `json:"created"`
		Token   *token       `json:"token"` // null while it has none
	}
	// What a Program is to be: its Name and Access level.
	type definition struct {
		Name  string       `json:"name"`
		Level access.Level `json:"level"`
	}
	by := func(r *http.Request) access.Identity {
		id, _ := identity(r)
		return id
	}
	mux.HandleFunc("GET /api/programs", func(w http.ResponseWriter, r *http.Request) {
		ps, err := acc.Programs(by(r))
		if err != nil {
			reply(w, err)
			return
		}
		list := make([]program, len(ps))
		for i, p := range ps {
			list[i] = program{p.ID, p.Name, p.Level, creator{p.Creator, acc.PersonName(p.Creator)}, p.Created, nil}
			if p.Token != nil {
				list[i].Token = &token{p.Token.Generated, p.Token.LastUse}
			}
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("POST /api/programs", func(w http.ResponseWriter, r *http.Request) {
		var req definition
		if !decode(w, r, &req) {
			return
		}
		p, err := acc.CreateProgram(by(r), req.Name, req.Level)
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": p.ID})
	})
	mux.HandleFunc("PUT /api/programs/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req definition
		if decode(w, r, &req) {
			reply(w, acc.EditProgram(by(r), r.PathValue("id"), req.Name, req.Level))
		}
	})
	mux.HandleFunc("DELETE /api/programs/{id}", func(w http.ResponseWriter, r *http.Request) {
		reply(w, acc.RemoveProgram(by(r), r.PathValue("id")))
	})
	// A new Token, shown this once; the previous one is revoked.
	mux.HandleFunc("POST /api/programs/{id}/token", func(w http.ResponseWriter, r *http.Request) {
		t, err := acc.GenerateToken(by(r), r.PathValue("id"))
		if err != nil {
			reply(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"token": t})
	})
	mux.HandleFunc("DELETE /api/programs/{id}/token", func(w http.ResponseWriter, r *http.Request) {
		reply(w, acc.RevokeToken(by(r), r.PathValue("id")))
	})
}
