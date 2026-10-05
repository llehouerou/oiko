# Starlark for the Code Step

The Code Step of an Automation runs Starlark (`go.starlark.net`), not JavaScript, even though the Node-RED function nodes it replaces were JavaScript. The engine runs in-process, one goroutine, against a state mirror so Runs are deterministic and replayable ([Where flows run](https://github.com/llehouerou/oiko/issues/14)); isolation from Home is the language's job, not the OS's. Starlark is hermetic by design (no I/O, clock or randomness unless the host provides them), has a deterministic step budget on top of cancellation, and a small, bounded set of builtins. A warm call on the representative handler (3.5 µs, 51 allocs) fits the ≤ 5 µs / 60 allocs budget ([Performance budget](https://github.com/llehouerou/oiko/issues/13)). Familiarity was the price: code written for Oiko is Python-like, and switching language later means rewriting every occupant's Code Steps.

## Considered Options

Findings: [runtime survey](https://github.com/llehouerou/oiko/blob/research/go-script-runtimes/docs/research/go-script-runtimes.md), [benchmark](https://github.com/llehouerou/oiko/blob/research/runtime-benchmark/docs/research/runtime-benchmark.md).

- **goja (JavaScript)**: familiar and as fast (3.7 µs, 45 allocs), but `Date.now`, `Math.random` and backtracking regexps must be stripped or tamed for determinism and safety, it stops only through Interrupt (no step budget), and costs 6–8 KiB per instance.
- **expr**: cheapest (1.1 µs), but expression-only and not interruptible; a stateful classifier (the Arlo alert set) doesn't fit.
- **gopher-lua**: over budget (6 µs, 89 allocs, 130 KiB per VM), and a quadratic `string.gsub` that no timeout stops.
- **Yaegi**: same language as Oiko, but not a hard security boundary.
- **wazero** (QuickJS or TinyGo in WASM): strongest isolation, but packaging and per-instance startup costs.
