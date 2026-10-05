package homekit

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/llehouerou/oiko/bridge"
)

// The HAP /accessories document, as far as Oiko reads it.
type accessories struct {
	Accessories []accessory `json:"accessories"`
}

type accessory struct {
	AID      uint64    `json:"aid"`
	Services []service `json:"services"`
}

type service struct {
	Type            string           `json:"type"`
	IID             uint64           `json:"iid"`
	Characteristics []characteristic `json:"characteristics"`
}

type characteristic struct {
	Type  string   `json:"type"`
	IID   uint64   `json:"iid"`
	Perms []string `json:"perms"`
	Value any      `json:"value"`
}

// shortType turns an Apple-defined UUID into its short form ("86"); other
// types are kept whole.
func shortType(t string) string {
	t = strings.ToUpper(t)
	if s, ok := strings.CutSuffix(t, "-0000-1000-8000-0026BB765291"); ok {
		return strings.TrimLeft(s, "0")
	}
	return t
}

// Function kind of each service type Oiko reads.
var kinds = map[string]string{
	"86": "occupancy",   // Occupancy Sensor
	"84": "illuminance", // Light Sensor
}

// A characteristic Oiko reads, as a Capability, and how its HAP value maps
// to a Value.
type reading struct {
	capability bridge.Capability
	value      func(any) (any, bool)
}

var readings = map[string]reading{
	"71": { // Occupancy Detected: 0 or 1
		bridge.Capability{Key: "occupancy", Label: "Occupancy", Type: bridge.Binary, Access: bridge.Access{Observable: true}, Category: bridge.Primary},
		func(v any) (any, bool) { f, ok := v.(float64); return f != 0, ok },
	},
	"6B": { // Current Ambient Light Level
		bridge.Capability{Key: "illuminance", Label: "Illuminance", Type: bridge.Numeric, Unit: "lx", Access: bridge.Access{Observable: true}, Category: bridge.Primary},
		func(v any) (any, bool) { f, ok := v.(float64); return f, ok },
	},
}

// binding is where a characteristic's values go.
type binding struct {
	function string
	reading
}

// described is a paired accessory as Oiko sees it: the Device, and the
// binding of each characteristic it reads, by IID.
type described struct {
	device   bridge.Device
	bindings map[uint64]binding
	events   []uint64 // IIDs of the bound characteristics that notify
}

// describe builds the Device of accessory id from its /accessories document,
// with the Values that document carries. Each service of a known kind is a
// Function, keyed by its kind, then by kind and IID for later ones of the
// same kind (an FP2's detection zones).
// ponytail: only accessory 1, the accessory itself; HomeKit bridges carrying
// several accessories would need one Device per aid.
func describe(id string, raw []byte) (described, []bridge.Reading, error) {
	var doc accessories
	if err := json.Unmarshal(raw, &doc); err != nil {
		return described{}, nil, err
	}
	i := slices.IndexFunc(doc.Accessories, func(a accessory) bool { return a.AID == 1 })
	if i < 0 {
		return described{}, nil, fmt.Errorf("homekit: %s describes no accessory 1", id)
	}
	services := slices.Clone(doc.Accessories[i].Services)
	slices.SortFunc(services, func(a, b service) int { return cmp.Compare(a.IID, b.IID) })

	var chars []characteristic
	d := described{device: bridge.Device{NativeAddress: id, Name: id}, bindings: map[uint64]binding{}}
	for _, s := range services {
		if shortType(s.Type) == "3E" { // Accessory Information
			for _, c := range s.Characteristics {
				v, _ := c.Value.(string)
				switch shortType(c.Type) {
				case "23":
					d.device.Name = v
				case "20":
					d.device.Vendor = v
				case "21":
					d.device.Model = v
				}
			}
			continue
		}
		kind, ok := kinds[shortType(s.Type)]
		if !ok {
			continue
		}
		fn := bridge.Function{Key: kind, Kind: kind}
		if slices.ContainsFunc(d.device.Functions, func(f bridge.Function) bool { return f.Key == kind }) {
			fn.Key = kind + "/" + strconv.FormatUint(s.IID, 10)
		}
		chars = append(chars, s.Characteristics...)
		for _, c := range s.Characteristics {
			r, ok := readings[shortType(c.Type)]
			if !ok {
				continue
			}
			fn.Capabilities = append(fn.Capabilities, r.capability)
			d.bindings[c.IID] = binding{fn.Key, r}
			if slices.Contains(c.Perms, "ev") {
				d.events = append(d.events, c.IID)
			}
		}
		if len(fn.Capabilities) > 0 {
			d.device.Functions = append(d.device.Functions, fn)
		}
	}
	return d, d.values(chars), nil
}

// values turns characteristic values into Readings, skipping unbound or
// malformed ones.
func (d described) values(chars []characteristic) []bridge.Reading {
	var rs []bridge.Reading
	for _, c := range chars {
		b, ok := d.bindings[c.IID]
		if !ok {
			continue
		}
		if v, ok := b.value(c.Value); ok {
			rs = append(rs, bridge.Reading{Function: b.function, Capability: b.capability.Key, Data: v})
		}
	}
	return rs
}
