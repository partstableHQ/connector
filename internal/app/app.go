// Package app wires the Wails desktop app: embedded assets, main window,
// single-instance lock, lifecycle.
package app

import (
	"fmt"
	"os"

	"github.com/partstableHQ/connector"
	"github.com/partstableHQ/connector/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Run opens the main window and runs the event loop until the app quits.
func Run() {
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
	})

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
