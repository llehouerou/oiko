package homekit

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/mdns"

	"github.com/llehouerou/oiko/bridge"
	"github.com/llehouerou/oiko/bridge/store"
)

// pair is the pair command, `oiko <bridge> pair [-id device-id] setup-code`:
// it pairs Oiko with the HomeKit accessory waiting on the network and keeps
// the pairing in the data directory, for Oiko to follow once restarted.
func pair(env bridge.Env, args []string) error {
	fs := flag.NewFlagSet("pair", flag.ContinueOnError)
	id := fs.String("id", "", "device ID of the accessory to pair, when several are waiting")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: oiko %s pair [-id device-id] setup-code\n", env.Name)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return errors.New("one setup code expected")
	}
	file := filepath.Join(env.DataDir, "pairings.json")
	var p Pairings
	if err := store.Load(file, pairingsFormat, &p); err != nil {
		return err
	}
	a, err := pairWaiting(&p, *id, fs.Arg(0))
	if err != nil {
		return fmt.Errorf("pairing: %w", err)
	}
	if err := store.Save(file, pairingsFormat, p); err != nil {
		return err
	}
	fmt.Printf("paired with %s; restart Oiko to follow it\n", a.ID)
	return nil
}

// pairWaiting finds the accessory waiting to be paired on the local network,
// or the one whose device ID is id when several are, and pairs Oiko with it
// using its setup code. It gives Oiko its controller identity on first use.
func pairWaiting(p *Pairings, id, code string) (Paired, error) {
	var waiting, paired []string
	var entry *mdns.ServiceEntry
	err := mdns.Discovery(mdns.ServiceHAP, func(e *mdns.ServiceEntry) bool {
		if !e.Complete() || id != "" && !strings.EqualFold(e.Info["id"], id) {
			return false
		}
		seen := fmt.Sprintf("%s (%s, %s)", e.Info["id"], e.Name, e.Info["md"])
		if e.Info["sf"] == "1" {
			waiting, entry = append(waiting, seen), e
		} else {
			paired = append(paired, seen)
		}
		return false // listen until the discovery times out
	})
	if err != nil {
		return Paired{}, err
	}
	switch {
	case len(waiting) == 0 && len(paired) == 0:
		return Paired{}, fmt.Errorf("no HomeKit accessory found on the network")
	case len(waiting) == 0:
		return Paired{}, fmt.Errorf("no HomeKit accessory waiting to be paired; already paired with a controller: %s", strings.Join(paired, ", "))
	case len(waiting) > 1:
		return Paired{}, fmt.Errorf("several accessories waiting to be paired, choose one with -id: %s", strings.Join(waiting, ", "))
	}
	e := entry
	if p.Controller.ID == "" {
		p.Controller = Controller{ID: hap.GenerateUUID(), Key: hex.EncodeToString(hap.GenerateKey())}
	}
	key, err := hex.DecodeString(p.Controller.Key)
	if err != nil {
		return Paired{}, err
	}
	c := &hap.Client{DeviceAddress: e.Addr(), DeviceID: e.Info["id"], ClientID: p.Controller.ID, ClientPrivate: key}
	err = c.Pair(e.Info["ff"], code)
	c.Close()
	if err != nil {
		return Paired{}, err
	}
	a := Paired{ID: c.DeviceID, Public: hex.EncodeToString(c.DevicePublic)}
	p.Accessories = append(p.Accessories, a)
	return a, nil
}
