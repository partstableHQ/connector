// Package telemetry holds the anonymous update-check ping — the ONLY
// telemetry the Connector sends (PRIVACY.md). The payload is exactly
// {app_version, os, arch, anonymous_install_uuid}: no part numbers, no
// queries, no email, no PII. Failures are silent by design; telemetry must
// never make the app less useful.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"time"
)

// DefaultCollectorURL is the self-hosted collector the ping posts to.
const DefaultCollectorURL = "https://partstable.com/api/updates/ping"

// EnvCollectorURL overrides the collector (development).
const EnvCollectorURL = "PARTSTABLE_TELEMETRY_URL"

// EnvOptOut force-disables the ping regardless of the Settings toggle.
const EnvOptOut = "PARTSTABLE_NO_TELEMETRY"

// Payload is the entire telemetry contract. Field names are part of the
// privacy promise documented in PRIVACY.md — do not add fields.
type Payload struct {
	AppVersion string `json:"app_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	InstallID  string `json:"anonymous_install_uuid"`
}

// NewPayload builds the payload for this machine.
func NewPayload(appVersion, installID string) Payload {
	return Payload{
		AppVersion: appVersion,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		InstallID:  installID,
	}
}

// CollectorURL resolves the collector endpoint.
func CollectorURL() string {
	if u := os.Getenv(EnvCollectorURL); u != "" {
		return u
	}
	return DefaultCollectorURL
}

// EnvForcedOptOut reports whether the environment disables telemetry.
func EnvForcedOptOut() bool {
	return os.Getenv(EnvOptOut) == "1"
}

// Ping sends the payload. Best-effort by contract: a failed or slow ping
// is an error for the caller to ignore, never a user-facing problem.
func Ping(ctx context.Context, collectorURL string, p Payload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, collectorURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return nil
}
