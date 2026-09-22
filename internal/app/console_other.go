//go:build !windows

package app

// AttachConsole is a Windows-only concern; on other platforms CLI verbs
// inherit the terminal naturally.
func AttachConsole() {}
