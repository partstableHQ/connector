// Command partstable is the PartsTable Connector binary: the desktop app by
// default, plus the CLI verbs (serve, doctor, login) that share the same core.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/partstableHQ/connector/internal/api"
	"github.com/partstableHQ/connector/internal/app"
	"github.com/partstableHQ/connector/internal/auth"
	"github.com/partstableHQ/connector/internal/doctor"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/update"
	"github.com/partstableHQ/connector/internal/version"
)

const usage = `partstable — the parts compendium, on your computer.

Usage:
  partstable          launch the desktop app
  partstable serve    local lookup API on 127.0.0.1 (no window)
  partstable doctor   diagnose install, database, and update health
  partstable login    sign in — opens your browser (free, no card)
  partstable login --api-key
                      paste an API key instead (headless machines)
  partstable logout   remove the stored key from this machine
  partstable update   check for updates and apply them (restart to finish)
  partstable version  print version information
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		app.Run()
		return
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Printf("partstable %s (commit %s)\n", version.Version(), version.Commit())
	case "doctor":
		os.Exit(runDoctor())
	case "serve":
		os.Exit(runServe())
	case "login":
		pasteKey := len(args) > 1 && (args[1] == "--api-key")
		os.Exit(runLogin(pasteKey))
	case "logout":
		os.Exit(runLogout())
	case "update":
		os.Exit(runUpdate())
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "partstable: unknown command %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
}

// runDoctor diagnoses this installation (FM-16) and exits nonzero only on
// hard failures — warnings mean "works, not yet set up".
func runDoctor() int {
	sum, err := doctor.Run(context.Background(), doctor.Options{}, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "partstable doctor: %v\n", err)
		return 1
	}
	if sum.Count(doctor.StatusFail) > 0 {
		return 1
	}
	return 0
}

// runServe exposes the local API without a window: the scripting and
// Docker path (FM-15). Loopback only; queries are never logged.
func runServe() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	port, err := api.Port()
	if err != nil {
		fmt.Fprintln(os.Stderr, "partstable serve:", err)
		return 1
	}
	srv := api.New(lookup.New(app.OpenCompendium()), version.Version(), port,
		auth.NewManager(auth.LoadConfig(), auth.NewKeyringStore()),
		update.NewManager(version.Version(), app.OpenAppDB()))
	l, err := srv.Listen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "partstable serve: cannot bind %s (%v) — free the port or set %s=<port>\n",
			srv.Addr(), err, api.EnvPort)
		return 1
	}
	fmt.Printf("partstable serve: http://%s — loopback only, no query logging; ctrl+c to stop\n", srv.Addr())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(l) }()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, "partstable serve:", err)
			return 1
		}
	case <-ctx.Done():
		_ = srv.Close()
	}
	return 0
}

// runLogin pairs this machine with a free account (FM-3): through the
// system browser by default, or with a pasted API key on headless
// machines. The key lands in the OS keychain either way.
func runLogin(pasteKey bool) int {
	store := auth.NewKeyringStore()
	if pasteKey {
		in := bufio.NewReader(os.Stdin)
		fmt.Print("Paste your API key: ")
		key, _ := in.ReadString('\n')
		key = strings.TrimSpace(key)
		if key == "" {
			fmt.Fprintln(os.Stderr, "partstable login: no key entered")
			return 1
		}
		fmt.Print("Account email (optional — enter to skip): ")
		email, _ := in.ReadString('\n')
		if err := store.Save(auth.KeyInfo{Email: strings.TrimSpace(email), APIKey: key}); err != nil {
			fmt.Fprintln(os.Stderr, "partstable login:", err)
			return 1
		}
		fmt.Println("API key stored in the OS keychain.")
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), auth.PairTimeout)
	defer cancel()
	info, err := auth.Pair(ctx, auth.LoadConfig(), nil, func(s string) { fmt.Println(s) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "partstable login:", err)
		return 1
	}
	if err := store.Save(info); err != nil {
		fmt.Fprintln(os.Stderr, "partstable login:", err)
		return 1
	}
	fmt.Printf("Signed in as %s — key stored in the OS keychain.\n", info.Email)
	return 0
}

// runLogout removes the stored credential from this machine.
func runLogout() int {
	if err := (auth.KeyringStore{}).Clear(); err != nil {
		fmt.Fprintln(os.Stderr, "partstable logout:", err)
		return 1
	}
	fmt.Println("Signed out — the key was removed from the OS keychain.")
	return 0
}

// runUpdate checks the release channel (sending the anonymous ping unless
// opted out) and stages a newer release. A restart finishes the update.
func runUpdate() int {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	m := update.NewManager(version.Version(), app.OpenAppDB())
	res, err := m.Check(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "partstable update:", err)
		return 1
	}
	fmt.Printf("Current version %s.", res.Current)
	if res.Latest == "" {
		fmt.Println(" No release has been published yet.")
	} else {
		fmt.Printf(" Latest release %s.\n", res.Latest)
	}
	switch {
	case res.PingSent:
		fmt.Println("Anonymous update ping sent (no part numbers, no PII — see PRIVACY.md).")
	case res.PingSkipped != "":
		fmt.Printf("Update ping skipped: %s.\n", res.PingSkipped)
	}
	if !res.Available {
		return 0
	}
	fmt.Println("Downloading…")
	v, err := m.Apply(ctx)
	if errors.Is(err, update.ErrUpToDate) {
		fmt.Println("Already on the latest version.")
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "partstable update:", err)
		return 1
	}
	fmt.Printf("Downloaded %s — restart partstable to finish the update.\n", v)
	return 0
}
