# Privacy

PartsTable Connector is built to be useful without watching you.

## What is collected

**One thing: the update check.** When the app checks for a new release
(automatically and when you click "Check for updates"), the request carries:

- `app_version` — e.g. `v0.1.0`
- `os` and `arch` — e.g. `windows/amd64`
- `anonymous_install_uuid` — a random ID generated on first install

That is the entire payload. It doubles as anonymous usage counting (an
install that checks for updates is an install in use), which lets us decide
what to build next without touching your data.

## What is never collected

- Part numbers or anything you paste into the app — every lookup runs
  against the local database on your machine and never leaves it.
- Your email or account identity (the update ID is random and not linked to
  your sign-in).
- Crash dumps with personal data. Crash reporting is **opt-in, off by
  default**, and sanitized stack traces only.

## Opting out

Two ways, both honored:

- Settings → turn off update checks (you will need to update manually).
- Environment variable `PARTSTABLE_NO_TELEMETRY=1`.

The local SQLite database lives in your user profile (path shown by
`partstable doctor`), contains zero PII, and is removed by uninstalling.
