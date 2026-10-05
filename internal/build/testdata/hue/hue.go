// Package hue is a type of Bridge relaying nothing, with a ping command
// answering pong.
package hue

import (
	"context"
	"fmt"
	"time"

	"github.com/llehouerou/oiko/bridge"
)

func init() {
	bridge.Register(bridge.Module{Type: "hue",
		New: func(bridge.Env) (bridge.Bridge, error) { return hue{}, nil },
		Commands: map[string]func(bridge.Env, []string) error{"ping": func(bridge.Env, []string) error {
			fmt.Println("pong")
			return nil
		}}})
}

type hue struct{}

func (hue) Run(context.Context, bridge.Port)                         {}
func (hue) Send(string, string, map[string]any, time.Duration) error { return nil }
