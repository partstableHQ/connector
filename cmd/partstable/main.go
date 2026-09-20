// Command partstable is the PartsTable Connector binary: the desktop app by
// default, plus the CLI verbs (serve, doctor, login) that share the same core.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/partstableHQ/connector/internal/app"
	"github.com/partstableHQ/connector/internal/doctor"
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
	case "serve", "login":
		// Honest stubs: each lands in its own slice (see ROADMAP.md).
		// Never pretend a feature exists before it does.
		fmt.Fprintf(os.Stderr, "partstable %s: not built yet — see ROADMAP.md\n", args[0])
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
