package build

import (
	"reflect"
	"runtime/debug"
	"testing"

	"github.com/llehouerou/oiko/bridge"
)

var types = []bridge.Registered{
	{Type: "hue", Package: "example.com/oiko-hue"},
	{Type: "sonos", Package: "example.com/oiko/sonos/bridge"},
	{Type: "zigbee2mqtt", Package: Oiko + "/internal/zigbee2mqtt"},
}

func TestRead(t *testing.T) {
	for _, c := range []struct {
		name    string
		bi      debug.BuildInfo
		stamped string
		want    Build
	}{
		{
			"released Oiko built by oiko-build",
			debug.BuildInfo{Main: debug.Module{Path: "oiko-build", Version: "(devel)"}, Deps: []*debug.Module{
				{Path: Oiko, Version: "v0.2.0"},
				{Path: "example.com/oiko-hue", Version: "v1.2.0"},
				{Path: "example.com/oiko", Version: "v0.1.0"},
				{Path: "example.com/oiko/sonos", Version: "v0.3.0-0.20261004163119-85cb02bc0dfc"},
			}},
			"",
			Build{"v0.2.0", []Type{
				{"hue", false, "example.com/oiko-hue", "example.com/oiko-hue", "v1.2.0"},
				{"sonos", false, "example.com/oiko/sonos/bridge", "example.com/oiko/sonos", "v0.3.0-0.20261004163119-85cb02bc0dfc"},
				{"zigbee2mqtt", true, Oiko + "/internal/zigbee2mqtt", Oiko, "v0.2.0"},
			}},
		},
		{
			"checkout built by oiko-build, types given by directory",
			debug.BuildInfo{Main: debug.Module{Path: "oiko-build", Version: "(devel)"}, Deps: []*debug.Module{
				{Path: Oiko, Version: "v0.0.0", Replace: &debug.Module{Path: "/src/oiko"}},
				{Path: "example.com/oiko-hue", Version: "v0.0.0", Replace: &debug.Module{Path: "/src/oiko-hue"}},
			}},
			"",
			Build{"", []Type{
				{"hue", false, "example.com/oiko-hue", "example.com/oiko-hue", ""},
				{"sonos", false, "example.com/oiko/sonos/bridge", "", ""},
				{"zigbee2mqtt", true, Oiko + "/internal/zigbee2mqtt", Oiko, ""},
			}},
		},
		{
			"development build: Nix, or a checkout without its history",
			debug.BuildInfo{Main: debug.Module{Path: Oiko, Version: "(devel)"}},
			"",
			Build{"", []Type{
				{"hue", false, "example.com/oiko-hue", "", ""},
				{"sonos", false, "example.com/oiko/sonos/bridge", "", ""},
				{"zigbee2mqtt", true, Oiko + "/internal/zigbee2mqtt", Oiko, ""},
			}},
		},
		{
			"Nix build of a release, stamped",
			debug.BuildInfo{Main: debug.Module{Path: Oiko, Version: "(devel)"}},
			"v0.3.0",
			Build{"v0.3.0", []Type{
				{"hue", false, "example.com/oiko-hue", "", ""},
				{"sonos", false, "example.com/oiko/sonos/bridge", "", ""},
				{"zigbee2mqtt", true, Oiko + "/internal/zigbee2mqtt", Oiko, "v0.3.0"},
			}},
		},
		{
			"checkout built by go build: Go's version wins over a stamp",
			debug.BuildInfo{Main: debug.Module{Path: Oiko, Version: "v0.0.0-20261004163119-85cb02bc0dfc"}},
			"v0.3.0",
			Build{"v0.0.0-20261004163119-85cb02bc0dfc", []Type{
				{"hue", false, "example.com/oiko-hue", "", ""},
				{"sonos", false, "example.com/oiko/sonos/bridge", "", ""},
				{"zigbee2mqtt", true, Oiko + "/internal/zigbee2mqtt", Oiko, "v0.0.0-20261004163119-85cb02bc0dfc"},
			}},
		},
	} {
		if got := Read(&c.bi, c.stamped, types); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}
