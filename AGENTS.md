# AGENTS.md

- Everything committed to this repo is written in English: code, comments, docs, ADRs, commit messages, and test data too (names of devices, rooms, Flags, Automations, notification texts in fixtures and tests). Conversation with the user may be in another language; the repo never is.
- This repo is public: never commit personal data. No real names of people, addresses or coordinates of a real home, its time zone, host names, IP or MAC addresses, device serials or IEEE addresses, account identifiers, tokens or other secrets, and no recording from a real installation (zigbee2mqtt captures, cloud API responses, Node-RED or Home Assistant exports) unless every identifier and name in it has been replaced with made-up ones. Test data uses made-up names (Alice, Bob, Living room, Office) and a public place (Johannesburg in the timing tests, Paris in examples). The same applies to issues, commit messages and research notes.
- Use the vocabulary defined in `GLOSSARY.md`; record hard-to-reverse decisions in `docs/adr/`.
- Once Oiko is public (`v0.3.0` on), ADR 0019 overrides the global "no backward compatibility" rule: data Oiko owns is migrated, never broken; the `bridge` contract, the HTTP/WebSocket API, what Code Steps see and the configuration break only in a breaking release (a minor during v0, a major from `v1.0.0`), and a v0 patch neither breaks nor adds anything. Elsewhere, and before going public, the global rule holds.
- Web client change: see it rendered in the browser before reporting it; `docs/agents/ui-check.md` says how to reach the dev server and sign in.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (llehouerou/oiko), via `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` + `docs/adr/` at the root. See `docs/agents/domain.md`.
