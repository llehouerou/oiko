# Research: What Oiko must do itself to stay safe behind a reverse proxy that may add nothing but TLS

Scope: Oiko (Go stdlib `net/http`, Go 1.27, React SPA built by Vite, JSON API, SSE at `/api/updates`) exposed to the internet behind a reverse proxy run by the user. The proxy is assumed to terminate TLS and possibly do nothing else. This is research only. It lays out facts, options and trade-offs, and leaves the decisions to the project.

Labels used below: **[direct]** means the cited source says it. **[interp]** is my reading of a source. **[inference]** is my own reasoning and is not stated by any source.

Sources were consulted during this run. The Go 1.27 release notes are dated August 2026. Where a source's date or version matters, it is given inline.

---

## Key findings

1. **Go does not work out the client IP for you.** `Request.RemoteAddr` is the TCP peer, which is the proxy when one is in front. Go does not parse `X-Forwarded-For` or `Forwarded`. **[direct]** [request.go](https://raw.githubusercontent.com/golang/go/master/src/net/http/request.go)
   - Forwarded headers can only be trusted when they were added by proxies Oiko knows about, and only when Oiko cannot be reached directly. **[direct]** [MDN XFF](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Forwarded-For), [RFC 7239 §8.1](https://www.rfc-editor.org/rfc/rfc7239.txt)
   - The safe default is to trust no forwarded header at all. Trust should be an explicit opt-in that lists the proxy addresses.
   - A proxy that "adds nothing but TLS" makes every client look like one IP, usually loopback or a container bridge. **[inference]** So nothing may treat "loopback or private source IP" as trusted, and per-IP rate limits fall back to a single global bucket.
2. **`http.CrossOriginProtection` (Go 1.25+) is a solid CSRF layer for cookie-authenticated non-GET requests, including behind proxies.** **[direct]** [csrf.go](https://raw.githubusercontent.com/golang/go/master/src/net/http/csrf.go), [Filippo Valsorda, "Cross-Site Request Forgery"](https://words.filippo.io/csrf/)
   - It decides mainly from `Sec-Fetch-Site`, which browsers send to HTTPS origins. It falls back to comparing the `Origin` host with the `Host` header.
   - It does **not** protect:
     - GET, HEAD or OPTIONS requests (so state must never change on GET);
     - requests that carry neither header (non-browser clients);
     - XSS;
     - cross-origin reads;
     - WebSocket handshakes, which are GETs.
   - Go 1.25.0 had a bug (CVE-2025-47910) that made `AddInsecureBypassPattern` bypass more than intended. It is fixed in 1.25.1. **[direct]** [GO-2025-3955](https://pkg.go.dev/vuln/GO-2025-3955)
3. **The fallback path breaks behind proxies that rewrite `Host`.** nginx's default `proxy_set_header Host $proxy_host` is one example. **[direct]** [nginx](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)
   - The fallback only runs for browsers that do not send `Sec-Fetch-Site`, such as Safari/iOS before 16.4. **[direct]** [caniuse](https://caniuse.com/mdn-http_headers_sec-fetch-site)
   - With such a browser, every POST would be refused (403) unless the public origin is added with `AddTrustedOrigin`. **[interp]** An old wall tablet is the realistic case here.
4. **WebAuthn requires Oiko to know its public origin and RP ID exactly.** The RP **MUST NOT accept unexpected values of `origin`**. **[direct]** [WebAuthn L3 §13.4.9](https://www.w3.org/TR/webauthn-3/#sctn-validating-origin)
   - The most common Go library, go-webauthn, requires `RPID` and a non-empty `RPOrigins` list up front. **[direct]** [go-webauthn types.go](https://raw.githubusercontent.com/go-webauthn/webauthn/master/webauthn/types.go)
   - WebAuthn needs a secure context and a domain name, not an IP address. **[direct]** Same spec: `[SecureContext]` and "effective domain is not a valid domain → SecurityError".
   - **[inference]** A TLS-only proxy gives Oiko no reliable scheme or host signal. Learning the public URL from headers is therefore unreliable, and a **configured public URL** is the robust option.
5. **SSE cannot carry an `Authorization` header from `EventSource`.** The constructor only takes `withCredentials`. **[direct]** [HTML §9.2](https://html.spec.whatwg.org/multipage/server-sent-events.html)
   - A same-origin cookie is the natural credential.
   - Revoking a session needs active work on the server: cancel that session's open streams.
   - Once a stream is closed, the browser reconnects automatically. If the reconnect gets a non-200 response or a non-`text/event-stream` response, the browser **fails the connection and stops reconnecting**. **[direct]** Same spec. The client must notice this and send the user to login.
6. **Oiko's current production `index.html` contains no inline scripts or styles.** It only has `<script type="module" src=/assets/...>` and `<link rel=stylesheet href=/assets/...>`. **[direct, local file `web/dist/index.html`]**
   - So a strict `script-src 'self'` CSP needs no nonce or hash for the HTML.
   - React applies `style={...}` through the CSSOM (`node.style[x] = v`, `setProperty`), not through the `style` attribute. **[direct]** [React CSSPropertyOperations.js](https://raw.githubusercontent.com/facebook/react/main/packages/react-dom-bindings/src/client/CSSPropertyOperations.js)
   - The CSP spec ties `style-src-attr` to style **attributes**. **[direct]** [CSP3](https://w3c.github.io/webappsec-csp/)
   - **[interp]** So `style-src 'self'` without `'unsafe-inline'` is plausible. It still needs testing against `@xyflow/react` and `uplot` (Report-Only first).
7. **HSTS only takes effect when the browser receives it over HTTPS.** **[direct]** [MDN HSTS](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Strict-Transport-Security)
   - If Oiko sets it, it reaches the browser through the TLS proxy and works.
   - Browsers ignore it on direct plain-HTTP LAN access.
8. **Secure and `__Host-` cookies cannot be set from a plain-HTTP origin that is not "potentially trustworthy".** A LAN IP over `http://` is not trustworthy; `localhost` is. **[direct/interp]** [rfc6265bis-22 §4.1.3.2, App. A](https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-rfc6265bis-22)
   - **[inference]** Strong cookie settings and plain-HTTP LAN logins cannot both be had. This is a product decision.

---

## 1. Learning the client address

### Facts

- **Go's behaviour [direct].** `RemoteAddr` "allows HTTP servers … to record the network address that sent the request, usually for logging … The HTTP server in this package sets RemoteAddr to an "IP:port" address." ([request.go](https://raw.githubusercontent.com/golang/go/master/src/net/http/request.go)).
  - `net/http` has no client-IP helper and no trusted-proxy setting. **[interp]**
  - Go 1.27 adds none either: the release notes list no such change. ([Go 1.27 notes](https://go.dev/doc/go1.27))
- **`httputil.ReverseProxy` [direct].**
  - `NewSingleHostReverseProxy` uses the deprecated `Director` and "preserves X-Forwarded-\* headers sent by the client".
  - With `Rewrite`, `SetXForwarded` appends the client IP and sets `X-Forwarded-Host` and `X-Forwarded-Proto`. ([httputil](https://pkg.go.dev/net/http/httputil))
  - This matters only if Oiko itself proxies to something. It is also an example of the Go team treating client-supplied XFF as untrusted by default.
- **RFC 7239 `Forwarded` (June 2014), §8.1 [direct].**
  - The header "cannot be relied upon to be correct, as it may be modified … by every node on the way to the server, including the client making the request".
  - Allow-listing trusted proxies has weaknesses: "the chain of IP addresses listed before the request came to the proxy cannot be trusted".
  - The header has `for`, `by`, `host` and `proto` parameters. IPv6 values are quoted, for example `for="[2001:db8::17]:4711"`. ([RFC 7239](https://www.rfc-editor.org/rfc/rfc7239.txt))
- **MDN on X-Forwarded-For (last modified July 2025) [direct].**
  - "If the server can be directly connected to from the internet — even if it is also behind a trusted reverse proxy — **no part** of the X-Forwarded-For IP list can be considered trustworthy."
  - Multiple XFF headers must be joined into one list.
  - Two safe ways to select the client IP:
    - a trusted proxy *count*, counting from the rightmost entry;
    - a trusted proxy *list*, skipping entries from the right while they match the list.
  - "Leftmost (untrusted) values must only be used for cases where there is no negative impact from using spoofed values." ([MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Forwarded-For))
- **What common proxies do by default [direct]:**

  | Proxy | Default behaviour |
  |---|---|
  | Caddy `reverse_proxy` | Passes `Host` through. Sets or extends `X-Forwarded-For`. Sets `X-Forwarded-Proto` and `X-Forwarded-Host`. Ignores incoming values unless `trusted_proxies` is set. ([Caddy docs](https://caddyserver.com/docs/caddyfile/directives/reverse_proxy), [PR 4507](https://github.com/caddyserver/caddy/pull/4507)) |
  | nginx | Default `proxy_set_header Host $proxy_host`, i.e. the upstream name, not the browser's host. No XFF unless configured with `$proxy_add_x_forwarded_for`. ([nginx](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)) |
  | Traefik | Strips incoming `X-Forwarded-*` unless the source is in `forwardedHeaders.trustedIPs`; `insecure` is `false` by default. ([Traefik entrypoints](https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/)) |

  **[interp]** So depending on the proxy and its configuration, Oiko may see:
  - no forwarded headers at all;
  - XFF but a rewritten `Host`;
  - full `X-Forwarded-*`;
  - (rarely) RFC 7239 `Forwarded`.

### What this implies for Oiko [inference]

- **Default (no trusted proxies configured):** the client IP is the IP from `RemoteAddr`. Every forwarded header is ignored for security purposes; logging them as "claimed" values is fine.
  - This is safe when the proxy adds nothing.
  - Behind any proxy, every client then shares the proxy's IP.
- **Opt-in trusted-proxy configuration:** a list of CIDRs, or a hop count. Only when `RemoteAddr` is in the list, walk XFF (or `Forwarded: for=`) from the right, skipping trusted entries.
  - Supporting both XFF and `Forwarded` adds parsing surface. Picking one header per deployment (a config value) avoids mixing two inconsistent chains.
- **Where the client IP should be used:** audit logs, the "where you're signed in" session list, and login rate limiting or lockout. It should **not** be used for authorization ("LAN = trusted"). Behind a TLS-only proxy all internet traffic arrives from the proxy's IP, which is often loopback or RFC 1918.
- **Rate limiting when every client shares one IP:**
  - An IP-keyed limiter becomes global. One attacker can lock out the household (a DoS).
  - Options:
    - key on the account plus the (possibly global) IP;
    - prefer slowing responses down over hard lockouts;
    - make trusted-proxy config part of the documented setup.
- **Direct reachability:** if Oiko's port is also reachable without the proxy (for example a Docker port published on `0.0.0.0`), trusted headers can be spoofed by anyone who reaches it directly. Two levers: Oiko binding to a chosen address, and a documentation warning.

---

## 2. Cross-origin protection and CSRF

### What `http.CrossOriginProtection` does (source read from `master`) [direct]

From [csrf.go](https://raw.githubusercontent.com/golang/go/master/src/net/http/csrf.go):

1. `GET`, `HEAD` and `OPTIONS` are always allowed.
2. If `Sec-Fetch-Site` is `same-origin` or `none`, the request is allowed. Any other value is rejected, including **`same-site`** and `cross-site`, unless the request matches a trusted origin or bypass pattern.
3. With no `Sec-Fetch-Site` and no `Origin` header, the request is allowed ("either same-origin or not a browser request").
4. With no `Sec-Fetch-Site`: allowed if the `Origin` host equals `req.Host`, otherwise rejected.
   - The code comments that this fallback ignores the scheme and "fail[s] open" for HTTP→HTTPS. HSTS is the suggested mitigation.
5. Customisation hooks:
   - `AddTrustedOrigin("scheme://host[:port]")` uses exact match.
   - `AddInsecureBypassPattern` uses `ServeMux` patterns, exact match since 1.25.1 ([CVE-2025-47910 / GO-2025-3955](https://pkg.go.dev/vuln/GO-2025-3955)).
   - `SetDenyHandler` replaces the default 403 response.

Background and design rationale, by the author of the Go implementation ([words.filippo.io/csrf](https://words.filippo.io/csrf/)) [direct]:

- `Sec-Fetch-Site` is sent only to potentially trustworthy *targets*: HTTPS and localhost.
- It has been in all major browsers since 2023.
- An `Origin: null` must be treated as cross-origin.
- The only false positives are "requests to non-trustworthy (plain HTTP) origins that go through a reverse proxy that changes the Host header". The fix for those is to add the origin to the allow-list.
- Requests without browser headers "can't be affected by CSRF", so API traffic with bearer tokens does not need CSRF protection.

Browser support [direct, [caniuse](https://caniuse.com/mdn-http_headers_sec-fetch-site)]: Chrome 76, Firefox 90, Safari and iOS Safari 16.4.

### Behind the proxy [interp]

- **Public HTTPS access:** browsers send `Sec-Fetch-Site`, so the `Host` header is never consulted. The middleware works even when the proxy rewrites `Host` (nginx default) and even though Oiko itself sees plain HTTP.
- **Fallback path** (old browser, or a plain-HTTP origin): `Origin` is compared with the `Host` Oiko receives.
  - Through a `Host`-rewriting proxy, legitimate POSTs are refused.
  - Calling `AddTrustedOrigin(publicURL)` with the configured public origin fixes this.
  - This argues for a configured public URL (§3).
- **Direct LAN access over `http://`:** `Host` is not rewritten and `Origin` matches, so the request is allowed through the fallback.
  - **[inference]** Plain HTTP is still open to on-path attackers. CSRF is not the weak point there.

### What it does not cover [direct where cited, otherwise inference]

- **State changes on safe methods.** Every mutating endpoint must be POST, PUT, PATCH or DELETE. This includes logout, "acknowledge", "trigger scene" and "toggle" ([csrf.go doc comment](https://raw.githubusercontent.com/golang/go/master/src/net/http/csrf.go)).
- **Cross-origin *reads* and leaks.** Out of scope per Filippo. CORS and the absence of `Access-Control-Allow-Origin` handle reads. **[inference]** Oiko should not emit permissive CORS headers, especially with credentials.
- **WebSocket upgrades.** These are GETs, so the middleware lets them through. **[inference]** This is not relevant today (Oiko uses SSE), but a future WebSocket endpoint would need its own `Origin` check.
- **XSS.** Same-origin script passes every check. CSP (§5) is the mitigation.
- **Browser extensions that strip `Origin`.** Filippo calls this a vulnerability introduced by the extension.
- **Login CSRF.** Login is a POST, so it is covered, provided the login endpoint sits behind the middleware.

### Cookie-authenticated JSON endpoints [interp/inference]

Layers that combine:

1. `CrossOriginProtection` on the whole mux.
2. A session cookie with `SameSite=Lax` or `Strict`. This is defence in depth, *not* a cross-origin control: same-site sibling origins still pass ([Filippo](https://words.filippo.io/csrf/)).
3. Requiring `Content-Type: application/json` on JSON endpoints. That turns cross-origin `fetch` calls into CORS-preflighted requests, and `<form>` cannot produce the content type. **[interp]** Per Filippo this is "fairly limiting" as a sole defence, but it costs nothing for a JSON-only API.

Other notes:

- No CSRF token is needed with this design, per Filippo's algorithm.
- `SameSite=Strict` trade-off **[inference]**: the cookie is not sent on the first top-level navigation from another site, such as a link in a notification or chat app. The user would appear logged out until a reload.

### SSE and CSRF [interp/inference]

- `/api/updates` is a GET. CSRF (forging a state change) does not apply; leaking event data cross-origin does.
- A cross-site `EventSource` without `withCredentials` uses credentials mode "same-origin", so no cookie is sent ([HTML](https://html.spec.whatwg.org/multipage/server-sent-events.html)).
  - With `withCredentials`, the response is unreadable unless Oiko returns matching CORS headers.
  - `SameSite=Lax` cookies are not sent on cross-site subresource requests anyway.
- Defence-in-depth option: the web.dev "resource isolation policy" ([web.dev fetch-metadata](https://web.dev/articles/fetch-metadata)). Refuse a request when `Sec-Fetch-Site` is `cross-site`, unless it is a top-level `navigate` GET.
  - Applying this to `/api/*`, including `/api/updates`, would reject any cross-site fetch before authentication runs.

---

## 3. Host and origin: configured vs learnt public URL

### Facts

- **WebAuthn L3** (now a W3C Recommendation per its status section) ([spec](https://www.w3.org/TR/webauthn-3/)) [direct]:
  - The RP "MUST validate the origin member of the client data" and "MUST NOT accept unexpected values of origin".
  - The RP must verify that `rpIdHash` is SHA-256 of the RP ID *it expects*.
  - The API is `[SecureContext]`.
  - If the caller's effective domain "is not a valid domain", the call throws `SecurityError`. **[interp]** IP-address origins (`http://192.168.x.y`) cannot use passkeys; `localhost` is the secure-context exception.
- **go-webauthn** (`master`, [types.go](https://raw.githubusercontent.com/go-webauthn/webauthn/master/webauthn/types.go)) [direct]:
  - "At minimum, RPID and RPOrigins must be configured."
  - Validation fails if `RPOrigins` is empty.
  - Top-origin verification defaults to explicit mode.
- **Go `Request.Host`** is the `Host` header (HTTP/1) or `:authority` (HTTP/2) as received ([request.go](https://raw.githubusercontent.com/golang/go/master/src/net/http/request.go)) [direct]. Behind nginx's defaults it is the upstream name ([nginx](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)).
- **Cookies [direct]** ([rfc6265bis-22](https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-rfc6265bis-22); this is still an Internet-Draft, latest revision December 2025):
  - A `__Host-` cookie must have `Secure`, `Path=/` and no `Domain`. This "hews as closely as a cookie can to treating the origin as a security boundary", although ports are ignored.
  - `__Secure-`/`Secure` cookies are rejected unless set from a secure origin.
  - The draft "Considers potentially trustworthy origins as 'secure'".
  - **[interp]** A `__Host-` session cookie is host-only and needs no knowledge of the public hostname. It does need the *browser* to be on HTTPS, which the TLS proxy provides, even though Oiko sees HTTP.

### Configured vs learnt [interp/inference]

| | Configured public URL (e.g. `public_url = https://home.example.org`) | Learnt from `Host` / `X-Forwarded-Host` / `X-Forwarded-Proto` / `Forwarded` |
|---|---|---|
| TLS-only proxy | Works. | Scheme is wrong (Oiko sees `http`). Host may be the upstream name (nginx default). |
| Spoofing | Not affected by headers. | Only safe for headers from configured trusted proxies. Otherwise an attacker controls the RP ID and origin Oiko *expects*, and the hostname used in invite or reset links ("Host header injection"). |
| WebAuthn | Gives `RPID` and `RPOrigins` directly. RP ID must stay stable: changing it orphans every registered passkey. | RP ID could drift with whatever host the request used, and passkeys would silently stop working. |
| CSRF fallback | Feeds `AddTrustedOrigin`. | n/a |
| OIDC (optional) | Gives a stable `redirect_uri`. | Same spoofing risk. |
| UX | One more setup value. It can be pre-filled at first run from the request and confirmed by the admin. | Zero config. |

Options for several hostnames (LAN name plus public name):

- configure a list;
- or make "public URL" the single place where passkeys work.

go-webauthn and WebAuthn related origins can accept several origins under one RP ID ([spec §5.11](https://www.w3.org/TR/webauthn-3/)), but **[inference]** a separate LAN domain would need its own RP ID or a related-origins file.

**Host allow-list:** rejecting requests whose `Host` is neither the configured host nor an explicit LAN alias is cheap. It also blocks DNS-rebinding attempts against an Oiko port reachable on the LAN. **[inference]** DNS rebinding is mentioned as a network-position attack being addressed by Private Network Access ([Filippo, fn. 1](https://words.filippo.io/csrf/)). That proposal does not protect Oiko by itself.

---

## 4. Authenticating the long-lived SSE stream

### Facts [direct, [HTML Living Standard §9.2](https://html.spec.whatwg.org/multipage/server-sent-events.html)]

- `new EventSource(url, { withCredentials })` is the only configuration. There is no way to set request headers.
- `withCredentials: true` sets credentials mode `include`. Without it, same-origin requests still send cookies (credentials mode "same-origin").
- The browser reconnects after network errors, after a wait equal to the reconnection time (set by the `retry:` field), with optional backoff.
- If the response status is not 200, or `Content-Type` is not `text/event-stream`, the browser **fails the connection**: it ends in `readyState = CLOSED` with no further reconnects.
- A 204 response tells the client to stop reconnecting.
- Go side [direct, [net/http docs](https://pkg.go.dev/net/http#ResponseController)]:
  - `Request.Context()` is cancelled when the client disconnects or `ServeHTTP` returns.
  - `ResponseController.SetWriteDeadline` sets per-write deadlines. **[interp]** This lets a stream outlive a global `Server.WriteTimeout`.
- nginx buffers proxied responses by default (`proxy_buffering on`). The upstream can turn this off per response with `X-Accel-Buffering: no`.
- nginx `proxy_read_timeout` defaults to 60 s *between two reads*. **[interp]** Comment lines or heartbeats more often than every 60 s keep the stream alive behind default nginx. ([nginx](https://nginx.org/en/docs/http/ngx_http_proxy_module.html))

### Options [inference unless cited]

| Option | Pros | Cons |
|---|---|---|
| **A. Session cookie (same as the API)** | No extra token. Works with plain `EventSource`. `HttpOnly` cookie is never exposed to JS. | Cookie is checked once, at connect; revocation needs server-side stream tracking. |
| **B. Short-lived ticket in the query string** (`/api/updates?ticket=…`, minted by an authenticated POST) | Works for non-cookie clients. | Tokens in URLs end up in proxy access logs and history. Needs one-time use and a short TTL. |
| **C. `fetch()` + `ReadableStream` instead of `EventSource`** | Can send `Authorization` headers. | Reconnection and `Last-Event-ID` must be written by hand. Loses the native semantics. |
| **D. External programs** use bearer tokens on the same endpoint | Non-browser clients set headers freely. | Two auth paths on one endpoint. |

**Ending the stream on revocation (applies to A–D):**

1. Keep a registry of session ID → open streams, each with its own `context.CancelFunc`.
2. On logout, revocation, password or passkey removal, role downgrade or expiry, cancel those contexts. The handler returns and the stream closes.
3. The browser reconnects. The reconnect gets 401 or 403, so `EventSource` closes for good.
4. The SPA's `onerror` checks `readyState === EventSource.CLOSED` and sends the user to login.

Optional: send a final `event: session-ended` before closing, so the UI can explain why. Also re-check expiry on each heartbeat, so idle-timeout and absolute-timeout sessions end without an explicit revoke.

Cross-site reading of the stream is blocked by CORS plus `SameSite` (§2). Adding a `Sec-Fetch-Site` check for `/api/updates` is cheap defence in depth.

---

## 5. Security headers and CSP for the Vite-built SPA served by Go

### Facts

- **Built HTML** (`web/dist/index.html`, current local build) [direct]: one external module script with `crossorigin`, one external stylesheet, no inline `<script>`, no inline `<style>`, no `style=` attributes.
- **Dependencies** (from `web/package.json`) [direct]: React 19, `@xyflow/react` 12, `uplot` 1.6, `@dnd-kit/core`, `@mdi/js`, Tailwind 4 through `@tailwindcss/vite`, Vite 8.
- **Vite CSP guidance** ([Vite features → CSP](https://vite.dev/guide/features)) [direct]:
  - `html.cspNonce` adds a nonce placeholder. It needs a fresh nonce substituted into the HTML on every request; static nonces are a known pitfall ([vite#20531](https://github.com/vitejs/vite/issues/20531), [vite PR 20625](https://github.com/vitejs/vite/pull/20625)).
  - Assets under `build.assetsInlineLimit` are inlined as `data:` URIs, which need `data:` in `img-src`/`font-src`, or `assetsInlineLimit: 0`.
  - Never allow `data:` in `script-src`.
- **React styles** ([source](https://raw.githubusercontent.com/facebook/react/main/packages/react-dom-bindings/src/client/CSSPropertyOperations.js)) [direct]: client rendering applies the `style` prop through `node.style[name] = value` and `style.setProperty(...)`.
- **CSP3** ([editor's draft](https://w3c.github.io/webappsec-csp/)) [direct]:
  - `style-src-attr` "governs the behaviour of style attributes".
  - CSSOM `cssText` setters and `insertRule` are "gated on the `unsafe-eval` source expression" of `style-src`. The spec flags this as underspecified (issue #212).
  - `frame-ancestors` is ignored in a `<meta>` CSP, so it must be sent as a header.
  - **[unverified]** Whether today's browsers actually enforce the CSSOM `cssText`/`insertRule` gating was not checked.
- **HSTS** ([MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Strict-Transport-Security)) [direct]: ignored over HTTP. Can only be disabled over HTTPS with `max-age=0`. `includeSubDomains` covers every subdomain.
- **OWASP Secure Headers Project** proposed values ([best practices](https://owasp.github.io/www-project-secure-headers/best-practices/)) [direct, partial]:
  - HSTS `max-age=63072000; includeSubDomains`
  - `X-Frame-Options: deny`
  - `X-Content-Type-Options: nosniff`
  - CSP `default-src 'self'; form-action 'self'; base-uri 'self'; object-src 'none'; frame-ancestors 'none'; upgrade-insecure-requests`
  - `Referrer-Policy: no-referrer`
  - `Clear-Site-Data` (on logout)
  - COOP, COEP and CORP values (truncated in the extracted page; not fully read).

### Candidate policy, to be validated in `Content-Security-Policy-Report-Only` first [inference]

```
Content-Security-Policy:
  default-src 'self';
  script-src 'self';
  style-src 'self';
  img-src 'self' data:;
  font-src 'self';
  connect-src 'self';
  object-src 'none';
  base-uri 'none';
  form-action 'self';
  frame-ancestors 'none'
X-Content-Type-Options: nosniff
Referrer-Policy: no-referrer            (or strict-origin-when-cross-origin)
Cross-Origin-Opener-Policy: same-origin
Cross-Origin-Resource-Policy: same-origin
X-Frame-Options: DENY                   (legacy; frame-ancestors supersedes)
Permissions-Policy: camera=(), microphone=(), geolocation=()   (adjust to features used)
Strict-Transport-Security: max-age=...  (only effective via the TLS proxy; see open points)
Cache-Control: no-store                 (on /api/* responses carrying household data)
```

Notes on the candidate:

- **No nonce or hash needed** for the current build output.
  - Things that would change this: an inline theme or flash-prevention script, a `<style>` in `index.html`, or a library that injects `<style>` elements at runtime.
  - The last case would need `style-src 'self' 'unsafe-inline'`, a hash, or the nonce machinery.
- **`style-src 'self'` without `'unsafe-inline'`** is likely compatible with React's `style` prop. It is unverified for `@xyflow/react` and `uplot`: if either sets `style` via `setAttribute` or `cssText`, styles would be dropped.
- **`img-src data:`** is only needed if the build inlines assets (Tailwind/Vite inlining, `@mdi/js` paths are inline SVG `path` data in JSX, not `data:` URLs). It can be removed with `assetsInlineLimit: 0`.
- **`connect-src 'self'`** covers same-origin `fetch` and `EventSource`. Any external program or OIDC discovery is server-side and unaffected. If the SPA ever calls an OIDC provider from the browser, that origin must be added.
- **`upgrade-insecure-requests`** is harmless on the HTTPS public origin. It would break plain-HTTP LAN access if that is kept.
- **Trusted Types** (`require-trusted-types-for 'script'`) is an option. React 19 compatibility was not verified in this run.
- **Dev mode:** the Vite dev server injects inline scripts (React Refresh preamble), so production headers should not be applied to it.

---

## Contradictions

- **Is `SameSite` a CSRF defence?** Filippo argues it is "by design, not a cross-origin protection" and that Lax-by-default defaults "are not effective CSRF countermeasures" ([words.filippo.io/csrf](https://words.filippo.io/csrf/)). Many guides present `SameSite=Lax` as sufficient. These are compatible: it is defence in depth, not the primary control.
- **Fetch-metadata policies differ.** The web.dev resource isolation policy allows `same-site` ([web.dev](https://web.dev/articles/fetch-metadata)). Go's `CrossOriginProtection` rejects `same-site` for unsafe methods ([csrf.go](https://raw.githubusercontent.com/golang/go/master/src/net/http/csrf.go)). For Oiko, the stricter Go behaviour is the relevant one for mutations.
- **Choosing the XFF entry.** Some community guidance picks the leftmost XFF entry. MDN and adam-p say leftmost is spoofable and only right-side or trusted entries are fit for security use ([MDN](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Forwarded-For), [adam-p](https://adam-p.ca/blog/2022/03/x-forwarded-for/)).

## Missing evidence / not verified

- Whether current Chrome, Firefox and Safari enforce CSP3's `unsafe-eval` gating of CSSOM `cssText`/`insertRule`, and whether `@xyflow/react` or `uplot` use `setAttribute('style')`, `cssText` or injected `<style>` elements. This needs a Report-Only test.
- Whether the built CSS or JS contains `data:` URLs, or uses `eval`/`new Function` (`script-src` without `'unsafe-eval'`). Not grepped in this run.
- The Oiko server code (current handler wiring, SSE implementation, `Server` timeouts) was not inspected. The findings are framework-level.
- No Go proposal for a stdlib trusted-proxy or client-IP helper was searched beyond the Go 1.27 release notes.
- OWASP's full COOP, COEP and CORP recommendations were truncated in the extracted page.
- `rfc6265bis` is still an Internet-Draft (rev. -22, December 2025). Browser behaviour for prefixes and Secure-from-insecure is long-standing, but the normative text may still change.

## Open points for the decision

1. **Trusted-proxy model:**
   - none (default, `RemoteAddr` only);
   - CIDR list;
   - hop count;
   - and whether to support `Forwarded` (RFC 7239), XFF, or both.
2. **Public URL:**
   - required config;
   - first-run capture plus admin confirmation;
   - or learnt from trusted headers.
   - Related: one hostname or several (LAN alias), and the WebAuthn RP ID that must never change.
3. **Plain-HTTP LAN access:**
   - forbid it (HTTPS everywhere, `__Host-` cookies, passkeys work);
   - or allow an insecure LAN mode (non-Secure cookies, no passkeys, `Sec-Fetch-Site` absent so the Host fallback applies).
4. **Host allow-list** (reject unknown `Host`), for DNS-rebinding and Host-injection hardening.
5. **Login rate limiting** that stays useful when every client appears as one IP.
6. **SSE auth:**
   - cookie only (A);
   - plus a ticket or bearer path for external programs (B or D);
   - or a `fetch`-stream client (C).
   - Also: how revocation reaches open streams (registry plus cancel), and heartbeat interval (under 60 s for default nginx).
7. **`SameSite` value** (Lax vs Strict), weighed against links from notifications.
8. **CSP strictness:**
   - `style-src` with or without `'unsafe-inline'`;
   - `data:` in `img-src`;
   - Trusted Types;
   - Report-Only period and a reporting endpoint.
9. **HSTS:** should Oiko emit it (only effective through the TLS proxy), with what `max-age`, and never `includeSubDomains` by default? Or should the proxy own it?
10. **Bypass discipline:** any `AddInsecureBypassPattern` use (e.g. an OIDC callback, which is normally a GET anyway) needs Go ≥ 1.25.1 and an exact-match route.

## Sources

Kept:
- Go `net/http/csrf.go` (master): https://raw.githubusercontent.com/golang/go/master/src/net/http/csrf.go. The exact algorithm.
- Go `net/http/request.go`: https://raw.githubusercontent.com/golang/go/master/src/net/http/request.go. `RemoteAddr` and `Host` semantics.
- Go `net/http/httputil` docs: https://pkg.go.dev/net/http/httputil. X-Forwarded handling in Go's own proxy.
- Go 1.27 release notes: https://go.dev/doc/go1.27. No relevant `net/http` changes.
- GO-2025-3955 / CVE-2025-47910: https://pkg.go.dev/vuln/GO-2025-3955. Bypass-pattern bug fixed in 1.25.1.
- Filippo Valsorda, "Cross-Site Request Forgery": https://words.filippo.io/csrf/. Design rationale behind the Go middleware.
- RFC 7239: https://www.rfc-editor.org/rfc/rfc7239.txt. `Forwarded` syntax and security considerations.
- MDN X-Forwarded-For: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/X-Forwarded-For. Parsing and selection guidance.
- W3C WebAuthn Level 3: https://www.w3.org/TR/webauthn-3/. Origin and RP ID verification, secure context, valid domain.
- go-webauthn `types.go`: https://raw.githubusercontent.com/go-webauthn/webauthn/master/webauthn/types.go. Required `RPID`/`RPOrigins`.
- HTML Living Standard, server-sent events: https://html.spec.whatwg.org/multipage/server-sent-events.html. `EventSource` API, reconnect and fail semantics.
- draft-ietf-httpbis-rfc6265bis-22: https://datatracker.ietf.org/doc/html/draft-ietf-httpbis-rfc6265bis-22. `__Host-` prefix, Secure.
- CSP3 editor's draft: https://w3c.github.io/webappsec-csp/. `style-src-attr`, CSSOM gating, `frame-ancestors` in meta.
- Vite features, CSP: https://vite.dev/guide/features. Nonce and `data:` guidance.
- React `CSSPropertyOperations.js`: https://raw.githubusercontent.com/facebook/react/main/packages/react-dom-bindings/src/client/CSSPropertyOperations.js. Style applied via CSSOM.
- MDN HSTS: https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Strict-Transport-Security
- nginx proxy module: https://nginx.org/en/docs/http/ngx_http_proxy_module.html. Host default, buffering, read timeout.
- Caddy `reverse_proxy` docs: https://caddyserver.com/docs/caddyfile/directives/reverse_proxy. Header defaults.
- Traefik entrypoints: https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/. Forwarded-header trust defaults.
- caniuse `Sec-Fetch-Site`: https://caniuse.com/mdn-http_headers_sec-fetch-site. Browser versions.
- web.dev Fetch Metadata: https://web.dev/articles/fetch-metadata. Resource isolation policy.
- OWASP Secure Headers best practices: https://owasp.github.io/www-project-secure-headers/best-practices/. Baseline header values.

Deprioritised:
- Stack Overflow threads on CSP and React inline styles: anecdotal; replaced by the React source and the CSP spec.
- adam-p blog on XFF: a good secondary source, but MDN covers the same guidance.

## Next steps (research)

- Run the candidate CSP as Report-Only against the real SPA (graph editor with `@xyflow`, charts with `uplot`), and grep `dist/` for `data:`, `eval` and `new Function`.
- Read Oiko's current server wiring (mux, SSE handler, `http.Server` timeouts) to map each finding onto concrete handlers.
