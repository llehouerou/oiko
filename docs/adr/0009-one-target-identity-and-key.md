# One Target identity and key

Every Target (a Device's Function, the Device itself, an Aggregate or a Flag, and later a Group) is one sealed Go value, built only through constructors or by parsing its key, and one canonical string key everywhere it leaves Home: `device:<id>`, `device:<id>/<function>`, `aggregate:<id>`, `flag:<id>`. That key is what the API, the SSE stream, the web store, Starlark's `trigger.key`, the Command history and the JSON documents carry. Inside Home, one lookup resolves a Target to its kind; only how a Command reaches it differs from kind to kind. Before this, Target, Ref and Aggregate Member each held four optional ids, callers branched on which one was set, and five different string encodings of a target existed side by side. Adding Flags touched 16 files, and Group would have done the same. Function keys may contain `/` (`switch/l2`), but ids never do, so a key splits unambiguously on the first `/` after the id.

## Considered Options

- **A plain string type holding the key**: the simplest type, but nothing stops an arbitrary string from being passed as a Target, and every use has to parse it again to learn the kind.
- **Keeping the four-field struct on the wire and in files, and only centralising the dispatch inside Home**: no data or web change, but the five encodings and the three Availability maps stay, and so does the drift between them.

## Consequences

- Availability is keyed by the Target that owns it (a Device, an Aggregate or a Flag). A Function's Availability is its Device's, resolved by lookup, so one Device changing still announces one Update.
- An existing data directory is converted once, by a script that is never committed; the code reads only this shape.
