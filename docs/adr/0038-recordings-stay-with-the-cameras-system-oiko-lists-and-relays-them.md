# Recordings stay with the camera's system: Oiko lists and relays them

A camera's system may keep Recordings: the clips it recorded by itself, on motion or any trigger of its own (Arlo's library, an NVR's events). Oiko keeps none of them. A Bridge whose cameras have Recordings implements a second optional interface of the `bridge` contract, `bridge.Recordings`, beside `bridge.Cameras` (ADR 0036):

- `Recordings(ctx, address, function, from, to)` lists the Recordings of a camera Function that started within `[from, to]`, the newest first: for each, an id the Bridge chooses, opaque to Oiko and stable for as long as the system keeps it, when it started, how long it lasted, and what triggered it, a short lower-case word (`motion`, `person`, `sound`…) or nothing.
- `RecordingMedia(ctx, address, function, id, part, header)` fetches its video, an MP4 a browser plays, or its thumbnail, a still image, and returns the HTTP response: the Bridge sends the request with the headers Oiko passes on (`Range`, `If-Range`), whatever its system needs on top (credentials, its own certificate), and Oiko relays the answer to the browser, which plays the video and seeks in it with range requests. `bridge.ErrNotFound` tells Oiko an id the system no longer has.

The dashboard shows a camera's Recordings in the History of its Device, over the range shown: one marker each on the time scale, and their thumbnails, newest first, each playing its video when tapped. Recordings are the home's past: a Member or an Admin reads them, never a Guest (ADR 0023). Watching one is not recorded, unlike a Live view: it wakes no camera, and only those who already read the History see it.

## Considered Options

- **Oiko keeping the Recordings**, copying each one into its data directory: one list for every vendor and no expiry, but Oiko would become an NVR, with storage, retention and backups to manage, which ADR 0036 leaves to other systems.
- **Each Recording as an Event of the camera**, recorded in the History: Automations could follow them, but a Recording is then one more row Oiko keeps for something its system may delete, the Bridge would have to learn of each one as it is made, and the list would start at the release that brought it. A Bridge may still emit such an Event later; listing does not depend on it.
- **The Bridge returning a URL, which Oiko fetches**: simpler for a Bridge whose system serves presigned URLs (Arlo), but Oiko would then own the transport, so a system needing a session, a token in a header or a self-signed certificate could not be reached.
- **The browser fetching the system's URL itself**: no relay, but a presigned URL works for whoever holds it, a Guest's browser included, `media-src` would allow every vendor's host, and a system on the LAN is not reachable from outside.

## Consequences

- **Contract.** `bridge.Recordings`, `bridge.Recording`, `bridge.RecordingPart` (`video`, `thumbnail`) and `bridge.ErrNotFound` join the contract: a minor release during v0 (ADR 0019). `bridgetest` lists Recordings and reads their media.
- **API.** `GET /api/recordings?target=&from=&to=` lists them, at most the 1000 newest (ADR 0034); `GET /api/recordings/video` and `/api/recordings/thumbnail`, `?target=&id=`, relay their media, each write bounded as the event stream's. Both are GETs: listing or reading a Recording changes nothing and wakes no camera. A Bridge's error is logged, never answered: it may hold a presigned URL.
- **Cost.** Each listing and each fetch reaches the camera's system, through its Bridge, which may keep what it learnt for a while (Arlo's URLs stay valid a day); Oiko keeps nothing. The dashboard lists them again when the range moves, and every few minutes while it follows now.
- **Retention** is the system's: a Recording it deleted is gone from Oiko too.
