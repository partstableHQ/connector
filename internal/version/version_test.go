package version

import "testing"

// The version string must never be empty — it is shown in the window title
// bar, `partstable version`, the update check payload, and doctor output.
func TestVersionNeverEmpty(t *testing.T) {
	if Version() == "" {
		t.Fatal("Version() returned an empty string")
	}
	if Commit() == "" {
		t.Fatal("Commit() returned an empty string")
	}
	t.Logf("version=%s commit=%s", Version(), Commit())
}
