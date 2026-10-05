# Stable Device identity, independent of hardware and names

A Device has a Oiko-owned identifier assigned at discovery; its Native Address (Zigbee IEEE) is only a replaceable attribute, and its Name only a label. Every reference (Functions, Areas, future automations, history) goes through this identifier. Replacing a dead bulb means attaching the new Native Address to the existing Device: nothing else changes, and renaming or moving a Device breaks no reference.

Functions and Capabilities do not get their own identifiers: they are identified by a natural key within their Device, derived from the Bridge's description (Function: kind + endpoint, e.g. `switch/l2`; Capability: property name, e.g. `brightness`). A reference therefore reads `<device id>/switch/l2/on`. On Replace with the same model every key matches automatically; with a different model, matching keys are kept, new ones added and missing ones reported. A property renamed by the Bridge (e.g. z2m 2.0's `illuminance_lux` → `illuminance`) yields a new Capability; manual remapping can be added once references (automations, history) exist.

## Considered Options

- **Native Address as identity**: simpler, but every hardware replacement breaks all references (Home Assistant's `device_id` problem).
- **Human-readable name as identity** (`entity_id` style): readable references, but renaming or changing Area breaks them; Home Assistant made this worse by prefixing `entity_id`s with the area (~2026.6).
- **Oiko identifiers for Functions and Capabilities too**: survive any Bridge-side change, but Replace would need a separate matching step and references become fully opaque.
