# Cameras: a Picture and a Live view, relayed by Oiko

A camera is a `camera` Function of its Device, beside its other Functions (an Arlo camera's `occupancy` stays its own). What it sees is never a Value: a URL or an image in a Value would land in the History, in every Update and in what Automations read, and a presigned URL gives its media to whoever holds it. A camera instead offers two things through an optional interface of the `bridge` contract, `bridge.Cameras`, which only a Bridge with cameras implements:

- its **Picture**, the latest still image it has, with when it was taken, read without waking the camera;
- its **Live view**, opened on demand: the Bridge hands Oiko an `rtsp://` or `rtsps://` URL (`rtspx://` for RTSP over TLS whose certificate is not verified), fresh on each call or the same every time, and Oiko reads it.

A camera that streams on demand (Arlo, on battery, waking for each view) and one that streams permanently (a wired RTSP camera) are the same to Oiko; only what opening costs differs. Oiko relays each camera through one connection to the Bridge's URL, shared by everyone watching: it opens it for the first viewer and closes it 10 seconds after the last one leaves. Oiko never reads a camera nobody watches, and records nothing of what it sees: an NVR, such as Frigate, is another system, which may be a Bridge handing Oiko its own restream.

A Picture and a Live view are what the home looks like now, so a Guest sees them (ADR 0023). Each Live view is recorded in the camera's History, with its Origin, when it started and how long it lasted: being watched is something the camera went through, and the household may want to know who looked. Like a Command's Origin, it is read by a Member or an Admin only (ADR 0031), and only once the Live view ended.

## Considered Options

- **The stream URL or the Picture as a Value**: no new contract, but it puts secrets and bytes in the History and in every Update, and records each new URL as a change.
- **An external go2rtc server**, which Oiko would configure and proxy: less code in Oiko, but a second process with an unauthenticated API of its own, and Arlo's URL is new on each view, so go2rtc would have to call back into Oiko to open one. Oiko would still proxy every byte to sign it.
- **Each Bridge delivering what the browser plays**: the contract would carry finished media, but every type of Bridge would remux video itself and import a media library, which the stdlib-only contract cannot provide (ADR 0017).
- **Recording who watched in the Audit log**: it is kept a year and read by Admins, but the Audit log is about access, and watching a camera is about the camera.
- **Live views at Member and above only**, or chosen per camera by an Admin: more private, but a Guest already observes everything else in the home as it is now, and a per-camera rule is one more access concept. Kept for when a household needs it.

## Consequences

- **Relay.** It reads RTSP and writes fragmented MP4 with go2rtc's `pkg/rtsp` and `pkg/mp4`, already a dependency through HAP (ADR 0008): H.264 and AAC pass through, with no ffmpeg and no transcoding. A codec the browser cannot play is not converted.
- **Battery.** A camera whose Device reports a `battery` Capability shows a Live view for at most 5 minutes before the viewer must ask to keep watching; asking again while the connection is still open does not wake the camera twice. The dashboard closes a Live view when its page is hidden, and never starts one by itself: a camera's Tile shows its Picture, its age and a button to watch.
- **Memory.** At most 4 viewers per camera and 16 in all; one more is refused (ADR 0034).
- **Tiles.** The `camera` kind gives a Picture Tile (ADR 0014), placed and sized by the Layout like any other. A camera Function may have no Capability at all.
- **Pictures** are fetched from the Bridge when asked for, and kept in memory only for as long as Oiko serves them; a fresh Picture, which wakes the camera, is not part of this decision.
- **Recordings** a camera's vendor keeps (Arlo's library) are the home's past, for a Member and above: see ADR 0038.
- **HomeKit cameras** stream over HAP and SRTP, not RTSP: the built-in homekit Bridge would hand the relay a go2rtc source of its own, outside the contract.
