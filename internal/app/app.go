// Package app wires the Wails desktop app: embedded assets, main window,
// single-instance lock, the localhost API, lifecycle.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/partstableHQ/connector"
	"github.com/partstableHQ/connector/internal/api"
	"github.com/partstableHQ/connector/internal/auth"
	"github.com/partstableHQ/connector/internal/compendium"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/paths"
	"github.com/partstableHQ/connector/internal/store"
	"github.com/partstableHQ/connector/internal/update"
	"github.com/partstableHQ/connector/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Run opens the main window and runs the event loop until the app quits.
// The localhost API starts first: the UI and any local integrations speak
// the same verbs (FM-15).
func Run() {
	closeAPI := startLocalAPI()

	application.New(application.Options{
		Name:        "PartsTable Connector",
		Description: "The parts compendium, on your computer.",
		Assets: application.AssetOptions{
			Handler:        application.AssetFileServerFS(connector.FrontendFS),
			DisableLogging: true,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.partstable.connector",
		},
		OnShutdown: func() {
			if closeAPI != nil {
				closeAPI()
			}
		},
	})

	// A real menu bar with an explicit, keyboard-driven exit — closing
	// like a proper Windows citizen (CEO finding 2026-09-21). The X
	// button already quits the process.
	fileMenu := application.NewMenu().AddSubmenu("File")
	fileMenu.Add("Exit").
		SetAccelerator("Ctrl+Q").
		OnClick(func(*application.Context) { application.Get().Quit() })
	application.Get().Menu.SetApplicationMenu(fileMenu)

	application.Get().Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "PartsTable Connector " + version.Version(),
		Width:     1180,
		Height:    780,
		MinWidth:  940,
		MinHeight: 600,
	})

	if err := application.Get().Run(); err != nil {
		fmt.Fprintf(os.Stderr, "partstable: %v\n", err)
		os.Exit(1)
	}
}

// startLocalAPI brings up the loopback API over the installed compendium
// (if any) and returns a shutdown func. Every failure degrades honestly to
// a UI with no data — it never blocks the window.
func startLocalAPI() func() {
	port, err := api.Port()
	if err != nil {
		fmt.Fprintln(os.Stderr, "partstable:", err)
		return nil
	}
	// The app database opens and migrates eagerly at every startup
	// (BUILD-GUIDE §1); the update channel needs it for the install id
	// and the telemetry opt-out setting.
	authManager := auth.NewManager(auth.LoadConfig(), auth.NewKeyringStore())

	// Sign-in opens INSIDE the app — a focused window, impossible to lose
	// behind browser windows (CEO beta finding 2026-09-22). Headless runs
	// keep the system-browser fallback.
	var signInMu sync.Mutex
	var signInWin *application.WebviewWindow
	authManager.OpenSignInPage = func(u string) (func(), error) {
		signInMu.Lock()
		defer signInMu.Unlock()
		if signInWin != nil {
			signInWin.Close()
			signInWin = nil
		}
		signInWin = application.Get().Window.NewWithOptions(application.WebviewWindowOptions{
			Name:  "signin",
			Title: "PartsTable — Sign in",
			URL:   u,
			Width: 560, Height: 720,
		})
		return func() {
			signInMu.Lock()
			defer signInMu.Unlock()
			if signInWin != nil {
				signInWin.Close()
				signInWin = nil
			}
		}, nil
	}

	updateManager := update.NewManager(version.Version(), OpenAppDB())
	srv := api.New(lookup.New(OpenCompendium()), version.Version(), port, authManager, updateManager)
	// Outbound links open in the system browser, never inside the app
	// window. Resolved at click time, when the Wails app exists.
	srv.SetURLOpener(func(u string) error {
		return application.Get().Browser.OpenURL(u)
	})
	l, err := srv.Listen()
	if err != nil {
		fmt.Fprintf(os.Stderr, "partstable: local API could not bind %s (%v) — the UI will show no data; free the port or set %s=<port>\n",
			srv.Addr(), err, api.EnvPort)
		return nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "partstable: local API stopped: %v\n", err)
		}
	}()
	return func() {
		_ = srv.Close()
		<-done
	}
}

// OpenAppDB opens and migrates the application database eagerly at
// startup. Failures degrade to nil — features that need it report
// honestly rather than blocking the app.
func OpenAppDB() *sql.DB {
	path, err := paths.AppDB()
	if err != nil {
		return nil
	}
	db, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "partstable: app database unavailable (%v) — run `partstable doctor`\n", err)
		return nil
	}
	if err := store.Migrate(context.Background(), db); err != nil {
		_ = db.Close()
		fmt.Fprintf(os.Stderr, "partstable: app database migration failed (%v) — run `partstable doctor`\n", err)
		return nil
	}
	return db
}

// OpenCompendium opens the installed compendium read-only. A missing file
// is not an error — the service then reports honestly that there is no
// data yet. Returns nil when nothing usable is installed.
func OpenCompendium() *sql.DB {
	path, err := paths.CompendiumDB()
	if err != nil {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	db, err := compendium.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "partstable: compendium unreadable (%v) — run `partstable doctor`\n", err)
		return nil
	}
	return db
}
