# React SPA frontend over a single JSON + SSE API

The frontend is a React 19 SPA (React Compiler, Vite, Tailwind 4), built to static assets embedded in the Go binary, talking to the backend through one JSON API for commands and one SSE stream for changes. The roadmap includes rich clients (node-graph automation editor, drag-and-drop dashboards, history charts, wall tablets), which need a JSON API anyway and live natively in the React ecosystem (e.g. xyflow). Perceived latency is dominated by the Zigbee radio, not the UI framework, provided each tile subscribes only to its own Capabilities.

## Considered Options

- **Datastar + templ (hypermedia) with rich islands as web components**: minimal JS and server-driven rendering, but two backend adapters (HTML and JSON), every rich library wrapped as a web component, `unsafe-eval` required in CSP, and a young 1.0 (April 2026) with a history of breaking changes and features moved to a commercial Pro tier.
- **htmx 4**: same trade-offs as Datastar for rich clients.
