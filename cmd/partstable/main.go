// Command partstable is the PartsTable Connector binary: the desktop app by
// default, plus the CLI verbs (serve, doctor, login) that share the same core.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/partstableHQ/connector/internal/api"
	"github.com/partstableHQ/connector/internal/app"
	"github.com/partstableHQ/connector/internal/doctor"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/version"
)

const usage = `partstable — the parts compendium, on your computer.

Usage:
  partstable          launch the desktop app
  partstable serve    local lookup API on 127.0.0.1 (no window)
  partstable doctor   diagnose install, database, and update health
  partstable login    sign in and pair this machine to your free account
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
		// Honest stub: lands with the auth slice (see ROADMAP.md).
		fmt.Fprintln(os.Stderr, "partstable login: not built yet — see ROADMAP.md")
		os.Exit(2)
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
	srv := api.New(lookup.New(app.OpenCompendium()), version.Version(), port)
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
