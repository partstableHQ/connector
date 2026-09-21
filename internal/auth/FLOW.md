# Account pairing protocol — v1

The binding contract between the PartsTable identity service (to be built)
and the Connector's sign-in flow (`pair.go`). Authorization-code flow with
PKCE over the system browser, redirecting to a loopback HTTP listener
(RFC 8252 style). The user's email is entered on the website, never in the
app.

## Endpoints (production)

- Authorize: `https://partstable.com/oauth/authorize` (GET, browser)
- Token: `https://partstable.com/oauth/token` (POST, form-encoded)
- `PARTSTABLE_AUTH_BASEURL` overrides both as `<base>/oauth/…` for
  development and staging.

## 1. Authorize request

The app opens the system browser with:

| parameter | value |
|---|---|
| `response_type` | `code` |
| `client_id` | `connector-desktop` |
| `redirect_uri` | `http://127.0.0.1:<random-port>/callback` |
| `state` | 128-bit random, base64url |
| `code_challenge` | S256(verifier), base64url |
| `code_challenge_method` | `S256` |
| `scope` | `api` |

The `<random-port>` is whatever the app's local listener bound — the server
MUST accept any loopback port in `redirect_uri` (RFC 8252 §7.3). The
authorize page is where the user enters their email (or signs up — free,
no card).

## 2. Redirect to the loopback callback

On success the server redirects to:

    <redirect_uri>?code=<authorization-code>&state=<state>

The app validates `state` byte-for-byte; a mismatch is refused and the
code is never used. The callback page shows "Sign-in complete — you can
close this window."

Errors redirect with `error=` (`access_denied`, …); the app renders a
human-readable failure and stops. Authorization codes are single-use and
short-lived (server's choice; ≤ 10 minutes recommended).

## 3. Token exchange (app ↔ token endpoint, backchannel)

    POST /oauth/token
    Content-Type: application/x-www-form-urlencoded

    grant_type=authorization_code
    code=<authorization-code>
    redirect_uri=<same redirect_uri as step 1>
    client_id=connector-desktop
    code_verifier=<verifier>

The server MUST verify `S256(code_verifier) == code_challenge` from step 1
and that the code was issued to this `client_id` + `redirect_uri`.

### Success — `200 OK`

```json
{ "account_email": "user@example.com", "api_key": "pt_..." }
```

### Failure — `4xx`

```json
{ "error": "human-readable sentence" }
```

The app shows `error` verbatim (it is user-facing copy).

## 4. Storage and use (client side)

- `api_key` is stored in the OS keychain (service
  `partstable-connector`, account `api`) as JSON together with the email.
  Never in plain files (SECURITY.md).
- The key identifies the free account for future entitlements and the
  update channel. v1.0's compendium lookups run locally and do not require
  the key to be present — sign-in is requested, not forced (FM-17: the
  free app never nags).
- `partstable logout` (and Sign out in the app) deletes the keychain entry.

## Failure states (user-facing, from the client)

- State mismatch / missing code → "sign-in could not be verified — please
  try again".
- No browser → "sign in with `partstable login --api-key` instead".
- Timeout (5 min) → "sign-in timed out — nothing was changed".
- Token endpoint `error` → shown verbatim.

## Headless fallback

`partstable login --api-key` reads an API key from stdin and stores it in
the keychain without any browser involvement. Servers issue keys through
the same identity service (contract: the account page exposes a
"generate API key" action returning a `pt_...` key).
