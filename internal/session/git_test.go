package session

import "testing"

func TestParseChangedFiles(t *testing.T) {
	status := " M README.md\nA  internal/session/manager.go\nR  old.go -> new.go\n?? tmp.txt"
	files := parseChangedFiles(status)
	want := []string{"README.md", "internal/session/manager.go", "new.go", "tmp.txt"}
	if len(files) != len(want) {
		t.Fatalf("expected %d files, got %d: %#v", len(want), len(files), files)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("file %d: expected %q, got %q", i, want[i], files[i])
		}
	}
}
