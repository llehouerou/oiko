# AGENTS.md

- Everything committed to this repo is written in English: code, comments, docs, ADRs, commit messages. Conversation with the user may be in another language; the repo never is.
- Use the vocabulary defined in `CONTEXT.md`; record hard-to-reverse decisions in `docs/adr/`.
- Once Oiko is public (`v0.3.0` on), ADR 0019 overrides the global "no backward compatibility" rule: data Oiko owns is migrated, never broken; the `bridge` contract, the HTTP/WebSocket API, what Code Steps see and the configuration break only in a breaking release (a minor during v0, a major from `v1.0.0`), and a v0 patch neither breaks nor adds anything. Elsewhere, and before going public, the global rule holds.

## Agent skills

### Issue tracker

Issues live in GitHub Issues (llehouerou/oiko), via `gh`. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` + `docs/adr/` at the root. See `docs/agents/domain.md`.
