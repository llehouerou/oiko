# Expose

## Public URL

Every Oiko needs a Public URL to sign anyone in, even one only ever used at home: an HTTPS origin
with no path, where users reach it, as `{"publicUrl": "https://oiko.example.org"}` in
`data/config.json` or `services.oiko.publicUrl` on NixOS; anything else there stops Oiko from
starting, with the reason. It takes a domain name and a certificate, from Let's Encrypt through a
DNS-01 challenge, which needs no open port, or a Tailscale `ts.net` name. For LAN-only use,
split-horizon DNS points the name at the host from inside the home. Without a Public URL, Oiko
starts and runs the home, but only http://localhost offers sign-in, and anywhere else only a
Program's Token is served.

## Reverse proxy

Oiko serves plain HTTP (`-listen`, `:8080` by default): put a reverse proxy that terminates TLS in
front of it. TLS is all the proxy has to add, since Oiko trusts no forwarded header, but it must
not buffer responses under `/api`: the dashboard follows the home through an event stream,
`/api/updates`, and plays a camera's Live view as it arrives, `/api/live-view`. Floods are the
proxy's job too: Oiko limits no request rate (ADR 0034). A Program on the LAN may still call Oiko
on its listen address, but a Token sent over plain HTTP can be read by anyone on the network.

With [Caddy](https://caddyserver.com), which gets the certificate and passes both streams on as
they come:

```caddyfile
oiko.example.org {
	reverse_proxy localhost:8080
}
```

For DNS-01, Caddy needs the DNS provider's module and a `tls { dns … }` block.
