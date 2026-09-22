// Command accountd is the PartsTable account service (internal/auth/FLOW.md):
// free email+password accounts for the Connector's OAuth+PKCE sign-in.
// It binds loopback and speaks plain HTTP — TLS terminates at
// Cloudflare/Caddy in production, which forwards partstable.com/oauth/*
// to this process.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/partstableHQ/connector/internal/accounts"
	"github.com/partstableHQ/connector/internal/auth"
)

func main() {
	addr := envOr("ACCOUNTD_ADDR", "127.0.0.1:8942")
	dbPath := envOr("ACCOUNTD_DB", "accounts.db")

	store, err := accounts.Open(dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "accountd:", err)
		os.Exit(1)
	}
	defer func() { _ = store.Close() }()

	svc := accounts.New(store, auth.ClientID)
	fmt.Printf("accountd listening on http://%s (db %s)\n", addr, dbPath)
	server := &http.Server{
		Addr:              addr,
		Handler:           svc.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		fmt.Fprintln(os.Stderr, "accountd:", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
