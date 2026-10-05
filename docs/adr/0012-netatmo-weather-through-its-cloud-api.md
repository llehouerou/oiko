# Netatmo weather stations through Netatmo's cloud API

Oiko reads the Netatmo weather station through a Bridge of its own polling Netatmo's cloud API (`getstationsdata`, scope `read_station`) every 5 minutes, rather than through Home Assistant's netatmo integration. The station has no local API and no HomeKit: it uploads its measures to Netatmo every 10 minutes and nowhere else, so while the internet or Netatmo is down the Bridge is offline. Each module, the main one included, is a Device whose Native Address is its MAC address; each of its measures a Function (`temperature`, `humidity`, `co2`, `noise`, `pressure`, `rain`); a Value carries the time the module measured it, not the time Oiko polled. A module the station no longer hears is offline, and the stale measures and battery Netatmo still lists for it are not reported. The first poll is the Bridge's Replay.

Oiko authenticates as an app the owner creates on dev.netatmo.com, through `golang.org/x/oauth2`. Netatmo invalidates a refresh token once it has issued the next, so the token lives in one file in the data directory that Oiko rewrites before using a new one, and that only one Oiko instance uses. It is seeded once by hand from the app's token generator: Oiko has no redirect URI to receive an authorization code.

## Considered Options

- **Home Assistant's netatmo integration**: works today, but Oiko would depend on the platform it replaces.
- **Netatmo's webhooks**: push instead of polling, but they cover the security products only, and need a public HTTPS endpoint.
- **A Go client library**: the existing ones use the password grant, which Netatmo removed; one endpoint does not warrant porting pyatmo.
