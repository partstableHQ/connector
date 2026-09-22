# Deploying accountd (the sign-in service)

**DEPLOYED 2026-09-22 with CEO approval** — live at
`https://partstable.com/oauth/*`, verified end-to-end: the real Connector
pairing (auth.Pair) completed against production from the CEO's machine
(account `beta@partstable.com`, key issued `pt_gwzMJ…`). Re-verify any
time with the opt-in production smoke:

```
PROD_EMAIL=... PROD_PASSWORD=... \
  go test -tags prodsmoke -run TestProdSignInSmoke ./internal/accounts/ -v
```

Deployment is **pending CEO approval** — it adds a public credential
endpoint to partstable.com. Everything is built and tested (see
internal/accounts: the real Connector pairing was run against this
implementation end-to-end); the production touch itself is the only
remaining step and takes ~10 minutes.

## What gets deployed

One static Go binary + one SQLite file. No containers, no external
dependencies. It implements exactly the contract in internal/auth/FLOW.md:

- `GET /oauth/authorize` — sign-in / sign-up form (free account, no card)
- `POST /oauth/authorize` — creates or authenticates the account
  (bcrypt-hashed password; never stored or logged in the clear)
- `POST /oauth/token` — PKCE exchange, issues `{account_email, api_key}`
- Authorization codes are single-use with a 10-minute TTL.

## Steps (on partstable-merged-01)

1. Build for linux (CI or locally): `GOOS=linux GOARCH=amd64 go build -o accountd ./cmd/accountd`
2. Copy to the server: `/usr/local/bin/accountd`.
3. Data dir: `mkdir -p /var/lib/accountd` (accounts.db lives there —
   back it up like any database).
4. systemd unit `/etc/systemd/system/accountd.service`:

   ```ini
   [Unit]
   Description=PartsTable account service
   After=network.target

   [Service]
   User=root
   Environment=ACCOUNTD_ADDR=127.0.0.1:8942
   Environment=ACCOUNTD_DB=/var/lib/accountd/accounts.db
   ExecStart=/usr/local/bin/accountd
   Restart=on-failure

   [Install]
   WantedBy=multi-user.target
   ```

   `systemctl daemon-reload && systemctl enable --now accountd`
5. Caddy: inside the existing `partstable.com { ... }` site block, add
   ABOVE the catch-all handler:

   ```
   handle /oauth/* {
       reverse_proxy 127.0.0.1:8942
   }
   ```

   Then `caddy validate --config /etc/caddy/Caddyfile && systemctl reload caddy`.
6. Verify: `curl -s https://partstable.com/oauth/health` → `ok`.

## Rollback

Remove the `handle /oauth/*` block, reload Caddy,
`systemctl disable --now accountd`. The Connector's pre-flight
(added 2026-09-21) makes the app degrade to a clean one-second
"account service isn't live yet" message — no user-visible breakage.

## Related infrastructure finding

During beta testing (2026-09-21) partstable.com intermittently returned
Cloudflare 1033 (tunnel cannot resolve the origin). cloudflared is up;
the tunnel's origin connection (ingress → https://localhost:443) shows
intermittent "no recent network activity" timeouts. That flakiness
affects the whole domain, not just sign-in — worth a look by whoever
owns the tunnel config (protocol/IPv6 tuning), independent of this
deployment.
