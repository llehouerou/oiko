package home

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Rename sets a Device's Name. Names are labels only: nothing refers to them.
func (h *Home) Rename(id DeviceID, name string) error {
	name, err := ValidName(name)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		return ErrNotFound
	}
	renamed := *d
	renamed.Name = name
	h.devices[id] = &renamed
	return h.registryChanged()
}

// SetIcon sets the Icon of Device or Aggregate t; "" goes back to the
// default.
func (h *Home) SetIcon(t Target, icon string) error {
	if err := ValidIcon(icon); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if id := t.Device(); id != "" && t.Function() == "" {
		d, ok := h.devices[id]
		if !ok {
			return ErrNotFound
		}
		changed := *d
		changed.Icon = icon
		h.devices[id] = &changed
		return h.registryChanged()
	}
	if id := t.Aggregate(); id != "" {
		if err := h.handMade(id); err != nil {
			return err
		}
		changed := *h.aggregates[id]
		changed.Icon = icon
		h.aggregates[id] = &changed
		return h.aggregatesChanged()
	}
	return fmt.Errorf("%w: a device's or an aggregate's icon is set here, an area's with its name", ErrInvalid)
}

// ValidIcon refuses what cannot name an Icon. Like a Name it is a label only,
// and Oiko does not draw it: it is the name of a picture the web client has;
// "" is none.
func ValidIcon(icon string) error {
	if len(icon) > 50 || strings.Trim(icon, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
		return fmt.Errorf("%w: an icon is named with lowercase letters, digits and dashes", ErrInvalid)
	}
	return nil
}

// Delete forgets a Detached Device for good, and its History.
func (h *Home) Delete(id DeviceID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	d, ok := h.devices[id]
	if !ok {
		return ErrNotFound
	}
	if !d.Detached {
		return fmt.Errorf("%w: only a detached device can be deleted", ErrInvalid)
	}
	h.forget(d)
	h.emit(Update{Kind: TargetDeleted, Target: TargetDevice(id, "")})
	return h.registryChanged()
}

// Replace attaches the hardware of Device with to the Detached Device id:
// id keeps its identity, Name, Icon and Areas and takes over with's Native Address,
// description, Availability, Values, last Events and History; with disappears.
func (h *Home) Replace(id, with DeviceID) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	old, okOld := h.devices[id]
	fresh, okFresh := h.devices[with]
	if !okOld || !okFresh {
		return ErrNotFound
	}
	if !old.Detached || fresh.Detached {
		return fmt.Errorf("%w: replace a detached device with an attached one", ErrInvalid)
	}

	// Collect what the new hardware already reported, under the kept identity.
	type kept struct {
		into map[Ref]Value
		ref  Ref
		v    Value
		kind UpdateKind
	}
	var moved []kept
	for _, src := range []struct {
		m    map[Ref]Value
		kind UpdateKind
	}{{h.values, ValueChanged}, {h.lastEvents, EventOccurred}} {
		for ref, v := range src.m {
			if ref.Target.Device() == with {
				ref.Target = TargetDevice(id, ref.Target.Function())
				moved = append(moved, kept{src.m, ref, v, src.kind})
			}
		}
	}
	availability, hadAvailability := h.availability[with]

	h.forget(old)
	h.forget(fresh)
	replaced := *fresh
	replaced.inherit(old)
	h.devices[id] = &replaced
	h.byAddress[native{replaced.Bridge, replaced.NativeAddress}] = id
	if hadAvailability {
		h.availability[id] = availability
	}
	for _, k := range moved {
		k.into[k.ref] = k.v
	}

	h.emit(Update{Kind: DeviceReplaced, Target: TargetDevice(id, ""), Replaced: with})
	err := h.persist()
	h.emit(Update{Kind: DevicesChanged, Devices: h.deviceList()})
	h.emit(Update{Kind: AvailabilityChanged, Target: TargetDevice(id, ""), Availability: h.effectiveAvailability(id)})
	for _, k := range moved {
		h.emit(Update{Kind: k.kind, Ref: &k.ref, Value: &k.v, Moved: true})
	}
	return errors.Join(err, h.rederive()) // Aggregates follow their members
}

// forget removes every trace of a Device. Callers hold h.mu.
func (h *Home) forget(d *Device) {
	delete(h.devices, d.ID)
	if key := (native{d.Bridge, d.NativeAddress}); h.byAddress[key] == d.ID {
		delete(h.byAddress, key)
	}
	delete(h.availability, d.ID)
	for _, m := range []map[Ref]Value{h.values, h.lastEvents} {
		for ref := range m {
			if ref.Target.Device() == d.ID {
				delete(m, ref)
			}
		}
	}
	h.stopSending(d.ID)
	h.commands.fail(func(t Target) bool { return t.Device() == d.ID })
}

// ValidName trims a Name and checks its length.
func ValidName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", fmt.Errorf("%w: a name has 1 to 100 characters", ErrInvalid)
	}
	return name, nil
}

// registryChanged saves the registry and announces it, then re-derives the
// Aggregates. Callers hold h.mu.
func (h *Home) registryChanged() error {
	err := h.persist()
	h.emit(Update{Kind: DevicesChanged, Devices: h.deviceList()})
	return errors.Join(err, h.rederive())
}

// persist saves the registry. Callers hold h.mu.
func (h *Home) persist() error {
	return h.save(devicesFile, devicesFormat, h.deviceList())
}
