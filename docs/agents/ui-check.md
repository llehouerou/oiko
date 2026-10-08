# Checking a web client change in the browser

A change to `web/` is done once you have seen it rendered: look at it with the browser tools before reporting it.

## 1. The dev server

`make dev` runs Oiko on `:8080` and the web client with hot reload on `http://localhost:5180`, which proxies `/api` to `:8080`. Only `:5180` shows uncommitted changes: `:8080` serves the client built into the binary when it started.

If nothing answers on `:5180` (`curl -sf localhost:5180`), start it in the background: `nohup make dev > /tmp/oiko-dev.log 2>&1 &`. Done when `localhost:5180` answers.

## 2. Signing in

The Persons of the dev home sign in with Passkeys, which the agent's browser does not hold. Use the host command instead:

1. Try the saved session first: open `http://localhost:5180`, `state load oiko-dev`, reload. Signed in if the page shows a Dashboard and not the sign-in screen.
2. Otherwise, from the repo root, while the dev server runs: `go run ./cmd/oiko sign-in-link` lists the Persons; `go run ./cmd/oiko sign-in-link <id>` of an Admin prints a Sign-in link on `http://localhost:8080`, valid 15 minutes, once.
3. Open the link, `Continue`, then `Not now` for the Passkey. The Session cookie is `localhost`'s, so it holds on `:5180` too.
4. `state save oiko-dev`, for the next session.

The dev home is the user's own (`data/`, never committed): look, and change only what the check needs, such as a test Dashboard, putting it back after.

## 3. Looking

Open the screen the change touches on `:5180`, then `vision` for a screenshot. A narrow place is a custom Dashboard of many columns: a Section one column wide shows Tiles as narrow as a phone's. Done when every state the change touches (hover, hidden name, editor, narrow) has been seen.
