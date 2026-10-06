package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/llehouerou/oiko/internal/access"
	"github.com/llehouerou/oiko/internal/history"
)

// handleAudit serves the Audit log, never as an Update (ADR 0033): all of it
// to an Admin, Person or Program, and to every other signed-in Person the
// entries where they act or are acted upon; nothing to anyone else. Newest
// first, a page at a time: ?before=<id> reads on from entry id. An Admin
// filters by identity with ?kind=<kind>&id=<id>. An identity that still
// exists carries its current Name beside the recorded one when they differ.
func handleAudit(mux *http.ServeMux, acc *access.Store, hist *history.Store) {
	type party struct {
		access.Party
		Current string `json:"current,omitempty"`
	}
	type entry struct {
		history.AuditEntry
		Actor   party `json:"actor"`
		Subject party `json:"subject"`
	}
	mux.HandleFunc("GET /api/audit", func(w http.ResponseWriter, r *http.Request) {
		id, _ := identity(r)
		var q history.AuditQuery
		switch {
		case id.Level == access.Admin:
			q.Party = access.Party{Kind: access.Kind(r.URL.Query().Get("kind")), ID: r.URL.Query().Get("id")}
		case id.Kind == access.PersonKind:
			q.Party = access.Party{Kind: id.Kind, ID: id.ID}
		default:
			http.Error(w, fmt.Sprintf("%v: only a Person, or an Admin Program, reads the Audit log", access.ErrRefused), http.StatusForbidden)
			return
		}
		if b := r.URL.Query().Get("before"); b != "" {
			var err error
			if q.Before, err = strconv.ParseInt(b, 10, 64); err != nil {
				http.Error(w, "before: want an entry's id", http.StatusBadRequest)
				return
			}
		}
		es, err := hist.AuditLog(q)
		if err != nil {
			reply(w, err)
			return
		}
		names := acc.CurrentNames()
		now := func(p access.Party) party {
			if name, ok := names[p.Kind][p.ID]; ok && name != p.Name {
				return party{p, name}
			}
			return party{Party: p}
		}
		list := make([]entry, len(es))
		for i, e := range es {
			list[i] = entry{e, now(e.Actor), now(e.Subject)}
		}
		writeJSON(w, http.StatusOK, list)
	})
}
