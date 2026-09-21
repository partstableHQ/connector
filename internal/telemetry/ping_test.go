package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The ping payload is the entire privacy promise: exactly these four
// fields, nothing more.
func TestPingPayloadExactFields(t *testing.T) {
	var mu sync.Mutex
	var got map[string]any
	var gotType string
	done := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		close(done)
	}))
	defer ts.Close()

	p := NewPayload("v0.1.0", "uuid-1234")
	if err := Ping(context.Background(), ts.URL, p); err != nil {
		t.Fatalf("ping: %v", err)
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
	want := map[string]any{
		"app_version":            "v0.1.0",
		"os":                     runtime.GOOS,
		"arch":                   runtime.GOARCH,
		"anonymous_install_uuid": "uuid-1234",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("payload[%q] = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != 4 {
		t.Errorf("payload has %d fields, want exactly 4 (PRIVACY.md contract): %v", len(got), got)
	}
	if !strings.HasPrefix(gotType, "application/json") {
		t.Errorf("content type = %q", gotType)
	}
}

func TestPingFailureReturnsError(t *testing.T) {
	// A dead collector must return an error (the caller ignores it), never
	// panic or block.
	err := Ping(context.Background(), "http://127.0.0.1:1/ping", NewPayload("v0.1.0", "id"))
	if err == nil {
		t.Fatal("expected an error from an unreachable collector")
	}
}

func TestEnvForcedOptOut(t *testing.T) {
	t.Setenv(EnvOptOut, "")
	if EnvForcedOptOut() {
		t.Fatal("empty env must not force opt-out")
	}
	t.Setenv(EnvOptOut, "1")
	if !EnvForcedOptOut() {
		t.Fatal("PARTSTABLE_NO_TELEMETRY=1 must force opt-out")
	}
}
