# Sign in

Sign-in needs a [Public URL](expose.md), except on http://localhost.

## Sessions and Tokens

Every API request but signing in needs a Session, signed in on the dashboard, or a Program's
Token, sent as `Authorization: Bearer`; the dashboard's page itself is served to anyone.

## Claim, invite, Kiosks

While Oiko has no Admin, its log prints at each start a Setup link that claims it
(`journalctl -u oiko` on NixOS). The Admin then invites the household, a Sign-in link for each
Person, and creates a Program and its Token for each script or system that calls the API.

A shared screen, such as a wall tablet, signs in as a Kiosk: its sign-in page offers a QR code,
which an Admin scans from a signed-in phone and approves.

## Host recovery

The host is the last way back in, for an Admin who lost every Passkey: while Oiko runs,
`oiko -data <dir> sign-in-link` lists the Persons (Name, Access level, id), and
`oiko -data <dir> sign-in-link <id>` prints a Sign-in link for one, valid 15 minutes, whatever
their Access level. It talks to Oiko through `<dir>/sign-in-link.sock`, open only to Oiko's user;
on NixOS, `sudo -u oiko oiko -data /var/lib/oiko sign-in-link`.
