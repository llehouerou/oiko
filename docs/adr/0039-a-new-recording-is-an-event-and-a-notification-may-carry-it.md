# A new Recording is an Event of its camera, and a Notification may carry its video

A Bridge announces each new Recording (ADR 0038) as soon as its system has it, with an Event of the camera Function's `recording` Capability: Stateless, of type enum, its data what triggered the Recording (its `Trigger`, or `other` when it has none), reported as of the Recording's start. Like any Event, it is recorded in the History and starts the Runs of the Event triggers set on it, `motion` or `person` for instance.

A notify Step may ask for the video of the Recording that started its Run. Oiko then looks among the camera's Recordings for the one that started at the Event's time, within a few seconds, and sends the Notification to Telegram as that video, its title and message as the caption. If the Run did not start from a `recording` Event of a camera, the Step's Trace says so; if the video cannot be fetched or sent, Telegram gets the text alone, which notes it. The video goes from the camera's system to Telegram through Oiko, without being stored.

## Considered Options

- **The Recording's id as the Event's data**: no time matching, but an Event trigger fires on a set of values, which an id never repeats, and the History would count each id apart.
- **A trigger of its own for Recordings**, with the id in its Run: precise, but a second way to react to what a camera saw, beside the Events every other kind of Device uses.
- **A link to the Recording in the Notification** instead of its video: no upload, but Oiko's Public URL may not be reachable from where the phone is, it needs signing in, and Telegram would show no preview.
- **Polling the Recordings in Oiko**: no Bridge work, but every Oiko would list every camera's system every minute, while the Bridge usually learns of a new Recording by itself.

## Consequences

- **Contract.** The `recording` Capability (`bridge.RecordingEvent`) and its meaning are documented on `bridge.Recordings`; no method changes.
- **Notification.** A notify Step gains `video`. Its Trace shows which Recording it asked for. Telegram takes a video of up to 50 MB from a bot; a longer one is sent as text.
- **Sending** a video may take a minute; Notifications stay in order, one at a time.
