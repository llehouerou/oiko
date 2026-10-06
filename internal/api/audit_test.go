package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/llehouerou/oiko/internal/access"
)

type auditEntry struct {
	ID             int64
	Event          access.Event
	Actor, Subject struct{ Kind, ID, Name, Current string }
	Browser        string
}

func TestEachReadsTheAuditLogTheirLevelAllows(t *testing.T) {
	_, acc, hdl := handlerAt(t, nil)
	srv := httptest.NewServer(hdl)
	t.Cleanup(srv.Close)
	alice, aliceID := signedIn(t, acc)
	do := browser(t, srv, "http://localhost:8080")
	bob, bobs := invited(t, do, alice, "Bob", access.Member)
	read(t, do("PUT", "/api/persons/"+bob, `{"name":"Robert","level":"member"}`, alice), http.StatusNoContent)
	_, admin := withProgram(t, acc, aliceID, "Node-RED", access.Admin)
	_, member := withProgram(t, acc, aliceID, "Scripts", access.Member)

	log := func(get func(string) *http.Response, query string) []auditEntry {
		return decodeAs[[]auditEntry](t, get("/api/audit"+query), http.StatusOK)
	}
	asAlice := func(path string) *http.Response { return do("GET", path, "", alice) }
	// Claimed, Bob created, his link, his sign-in, renamed, two Programs and a Token each.
	all := eventually(t, "the Audit log", func() ([]auditEntry, bool) { es := log(asAlice, ""); return es, len(es) >= 10 })
	if len(all) != 10 || all[0].Event != access.TokenGenerated || all[len(all)-1].Event != access.PersonCreated {
		t.Fatalf("Alice reads %d entries: %+v", len(all), all)
	}
	for _, e := range all {
		if e.Subject.ID == bob && (e.Subject.Name == "Bob") != (e.Subject.Current == "Robert") {
			t.Errorf("an entry of Bob as %q, now %q; want his current Name beside the recorded one only when they differ", e.Subject.Name, e.Subject.Current)
		}
	}
	if es := log(asAlice, "?kind=person&id="+bob); len(es) != 4 {
		t.Errorf("Alice reads %d entries of Bob, want 4: created, link, signed in, renamed", len(es))
	}
	if es := log(asAlice, "?before="+strconv.FormatInt(all[4].ID, 10)); len(es) != 5 || es[0].ID != all[5].ID {
		t.Errorf("the page after the fifth entry: %+v", es)
	}

	bot := asProgram(t, srv, admin)
	if es := log(func(path string) *http.Response { return bot("GET", path, "") }, ""); len(es) != 10 {
		t.Errorf("an Admin Program reads %d entries, want them all", len(es))
	}
	// Bob reads his own, whatever he asks.
	asBob := func(path string) *http.Response { return do("GET", path, "", bobs) }
	if es := log(asBob, "?kind=person&id="+aliceID.ID); len(es) != 4 {
		t.Errorf("Bob reads %d entries, want his 4", len(es))
	}
	if resp := asProgram(t, srv, member)("GET", "/api/audit", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("a Member Program reads the Audit log: %d, want 403", resp.StatusCode)
	}
}
