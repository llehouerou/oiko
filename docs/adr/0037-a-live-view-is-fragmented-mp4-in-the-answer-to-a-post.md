# A Live view is fragmented MP4 in the answer to a POST

The dashboard opens a camera's Live view (ADR 0036) with a `POST`, whose answer is the stream itself: fragmented MP4, an initialization segment then fragments starting at a key frame, for as long as the viewer reads it. The web client reads the body with `fetch` and feeds it to a `MediaSource`, or to a `ManagedMediaSource` where only that exists (Safari on iPhone), attached to a `<video>` through a `blob:` URL; aborting the request ends the Live view. Opening a Live view wakes a camera and costs its battery, so it is a POST: nothing changes state on a GET, and `http.CrossOriginProtection` and the `SameSite=Strict` Session cookie guard it as they guard every Command (ADR 0034). It needs nothing the event stream does not already: one long answer through the same reverse proxy, a write deadline on each write, and no buffering by the proxy.

## Considered Options

- **WebRTC**: the lowest latency, and the only way to talk back through a camera, but its media travels over UDP, or over TCP on a port of its own, which a reverse proxy adding only TLS does not carry: every exposed Oiko would need a forwarded port or a TURN server. It does not carry AAC either, so Arlo's audio would be transcoded.
- **HLS**: played natively by every Safari, older iPhones included, but several seconds behind, polled with GETs once started, and go2rtc offers no HLS server outside its own `internal` packages: Oiko would write the segmenter and the playlists.
- **MJPEG**: an `<img>` plays it everywhere, but it means decoding the video into JPEGs, with ffmpeg, and it has no sound.
- **A WebSocket carrying the same fragments**, as go2rtc's player does: the same media, but a library and a second kind of connection to authenticate and to guard against cross-site use, for a stream that only ever flows one way.
- **MP4 progressively served to `<video src>`**: no player code, but the browser fetches it with a GET, and Safari does not play an MP4 that never ends.

## Consequences

- **Content Security Policy.** `media-src 'self' blob:` joins the policy of ADR 0034: a `MediaSource` is attached through a `blob:` URL, which `default-src 'self'` refuses.
- **Permissions Policy.** `autoplay=(self)` replaces the `autoplay=()` of ADR 0034: a camera on battery sends its first key frame seconds after the tap that asked for it, past the browser's user activation, and a muted Live view must still start by itself.
- **iPhones.** `ManagedMediaSource` exists from iOS 17.1; an older iPhone shows a camera's Picture but no Live view.
- **Sound** plays only once the viewer unmutes it: browsers start no media with sound by themselves.
- **Programs** may open a Live view the same way, with their Token; it is recorded with their Origin.
