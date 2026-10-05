package bridge

import "testing"

func TestPackageOf(t *testing.T) {
	for fn, want := range map[string]string{
		"example.com/oiko-hue.init.0":                            "example.com/oiko-hue",
		"github.com/llehouerou/oiko/internal/netatmo.init.func1": "github.com/llehouerou/oiko/internal/netatmo",
		"gopkg.in/oiko.v2/hue.(*hue).register":                   "gopkg.in/oiko.v2/hue",
		"gopkg.in/hue%2ev2.init.0":                               "gopkg.in/hue.v2",
		"main.init.0":                                            "main",
	} {
		if got := packageOf(fn); got != want {
			t.Errorf("packageOf(%q) = %q, want %q", fn, got, want)
		}
	}
}
