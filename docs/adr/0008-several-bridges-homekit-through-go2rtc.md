# Several Bridges; HomeKit through go2rtc's HAP client

Oiko talks to more than one Bridge: zigbee2mqtt, and a HomeKit controller of its own for Wi-Fi accessories that speak only HomeKit locally (the Aqara FP2 presence sensor). Each Device records the Bridge it comes from, by name (`zigbee2mqtt`, `homekit`), and its Native Address is unique within that Bridge only. Home keeps one online state per Bridge: a Bridge going offline makes only its own Devices' Availability unknown, and refuses Commands to them only. Each Bridge feeds Home through a port bound to its name, so a Bridge's full list of Devices detaches only its own missing ones. A Detached Device may be replaced by hardware from another Bridge.

The HomeKit Bridge is Oiko acting as a HomeKit controller over the local network (HAP over IP): no Apple hardware, no cloud. It uses the HAP client of go2rtc (`github.com/AlexxIT/go2rtc/pkg/hap`) for pairing, the encrypted session, reading `/accessories` and subscribing to characteristic events. Pairing is done once from the command line with the accessory's setup code; Oiko's controller keys and each paired accessory's public key are kept in the data directory. The Bridge itself is always online; each accessory's Availability is whether its session is up.

## Considered Options

- **One composite Bridge multiplexing zigbee2mqtt and HomeKit behind the single-Bridge Home**: no change to Home, but zigbee2mqtt going offline would blank the FP2's Availability, and one list of Devices would have to be merged from two sources.
- **Going through Home Assistant or Homebridge**: they pair with the FP2, but Oiko would depend on a second home automation platform to relay a sensor.
- **hkontrol/hkontroller**: a Go HAP controller, abandoned since 2023 and its repository gone.
- **Our own HAP client** on `golang.org/x/crypto`: no unusual dependency, but about a thousand lines of SRP, pair-verify and session encryption to maintain. go2rtc's package is maintained and used against real accessories; its API carries no stability promise, a risk bounded by the one package that imports it.
