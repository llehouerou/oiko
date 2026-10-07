package api

import (
	"net/http"
	"testing"
)

func TestAnAreasIconComesBackInItsUpdatesAndTheSnapshot(t *testing.T) {
	_, do := server(t)
	msgs := stream(t, do)
	next(t, msgs, "snapshot")
	// icon is the Icon of the only Area in an Update or a snapshot.
	icon := func(m map[string]any) any { return m["areas"].([]any)[0].(map[string]any)["icon"] }

	read(t, do("POST", "/api/areas", `{"name": "Office", "icon": "desk"}`), http.StatusNoContent)
	m := next(t, msgs, "areas")
	if got := icon(m); got != "desk" {
		t.Fatalf("created with an Icon: %v", got)
	}
	id := m["areas"].([]any)[0].(map[string]any)["id"].(string)
	if got := icon(next(t, stream(t, do), "snapshot")); got != "desk" {
		t.Errorf("in a snapshot: %v", got)
	}

	read(t, do("PUT", "/api/areas/"+id, `{"name": "Office", "icon": "bookshelf"}`), http.StatusNoContent)
	if got := icon(next(t, msgs, "areas")); got != "bookshelf" {
		t.Errorf("edited to another Icon: %v", got)
	}
	read(t, do("PUT", "/api/areas/"+id, `{"name": "Office"}`), http.StatusNoContent)
	if got := icon(next(t, msgs, "areas")); got != nil {
		t.Errorf("saved without an Icon: %v", got)
	}
	if got := icon(next(t, stream(t, do), "snapshot")); got != nil {
		t.Errorf("in a snapshot, without an Icon: %v", got)
	}
	read(t, do("PUT", "/api/areas/"+id, `{"name": "Office", "icon": "Desk lamp"}`), http.StatusBadRequest)
	read(t, do("POST", "/api/areas", `{"name": "Den", "icon": "../sofa"}`), http.StatusBadRequest)
}
