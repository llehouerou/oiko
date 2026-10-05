package bridge

import (
	"encoding/json"
	"testing"
)

func TestDecodeRefusesUnknownKeys(t *testing.T) {
	var c struct {
		Broker string `json:"broker"`
	}
	if err := (Env{Config: json.RawMessage(`{"broker": "mqtt://x"}`)}).Decode(&c); err != nil || c.Broker != "mqtt://x" {
		t.Errorf("known key: %v, %+v", err, c)
	}
	if err := (Env{Config: json.RawMessage(`{"borker": "mqtt://x"}`)}).Decode(&c); err == nil {
		t.Error("misspelt key decoded")
	}
	if err := (Env{}).Decode(&c); err != nil {
		t.Errorf("no section: %v", err)
	}
}

func TestPackageOf(t *testing.T) {
	for fn, want := range map[string]string{
		"example.com/oiko-hue.init.0":                            "example.com/oiko-hue",
		"github.com/llehouerou/oiko/internal/homekit.init.func1": "github.com/llehouerou/oiko/internal/homekit",
		"gopkg.in/oiko.v2/hue.(*hue).register":                   "gopkg.in/oiko.v2/hue",
		"gopkg.in/hue%2ev2.init.0":                               "gopkg.in/hue.v2",
		"main.init.0":                                            "main",
	} {
		if got := packageOf(fn); got != want {
			t.Errorf("packageOf(%q) = %q, want %q", fn, got, want)
		}
	}
}
