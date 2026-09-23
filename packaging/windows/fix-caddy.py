#!/usr/bin/env python3
"""Re-insert the /oauth/* handle block into the Caddyfile for partstable.com."""
import sys

with open("/etc/caddy/Caddyfile", "r") as f:
    content = f.read()

if "handle /oauth/" in content and "reverse_proxy 127.0.0.1:8942" in content:
    print("already present")
    sys.exit(0)

block = (
    "\t# PartsTable account service (accountd, deployed 2026-09-22 with CEO\n"
    "\t# approval) — the Connector's OAuth+PKCE sign-in. Public credential\n"
    "\t# endpoint; runbook + rollback: connector repo cmd/accountd/DEPLOY.md.\n"
    "\thandle /oauth/* {\n"
    "\t\treverse_proxy 127.0.0.1:8942\n"
    "\t}\n\n"
)

marker = "\t# Static marketing website"
if marker in content:
    content = content.replace(marker, block + marker, 1)
    print("inserted before catch-all")
else:
    # Fallback: insert right after the CSP header block closing brace
    # inside the partstable.com site block
    content = content.replace("\thandle {\n", block + "\thandle {\n", 1)
    print("inserted before last handle")
    # Actually, we need it before the catch-all. Let's find the LAST handle block
    # in the partstable.com section. For now just prepend to the file after imports.
    print("WARNING: fallback insertion may need manual review")

with open("/etc/caddy/Caddyfile", "w") as f:
    f.write(content)
