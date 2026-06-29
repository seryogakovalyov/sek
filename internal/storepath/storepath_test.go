package storepath

import (
	"path/filepath"
	"testing"
)

func TestResolveExplicitPathWins(t *testing.T) {
	got, err := Resolve(Options{
		ProjectDir:   "/project",
		DataDir:      "/data",
		ExplicitPath: "/custom/store.db",
		Global:       true,
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got != "/custom/store.db" {
		t.Fatalf("Resolve = %q, want %q", got, "/custom/store.db")
	}
}

func TestResolveGlobalUsesDataDir(t *testing.T) {
	got, err := Resolve(Options{
		ProjectDir: "/project",
		DataDir:    "/data",
		Global:     true,
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := filepath.Join("/data", "store.db")
	if got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveProjectLocalUsesProjectDir(t *testing.T) {
	got, err := Resolve(Options{ProjectDir: "/project"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := filepath.Join("/project", ".sek", "store.db")
	if got != want {
		t.Fatalf("Resolve = %q, want %q", got, want)
	}
}

func TestResolveProjectLocalRequiresProjectDir(t *testing.T) {
	if _, err := Resolve(Options{}); err == nil {
		t.Fatal("expected empty project-local project dir to fail")
	}
}

func TestRequiresProject(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want bool
	}{
		{name: "project local", opts: Options{}, want: true},
		{name: "global", opts: Options{Global: true}, want: false},
		{name: "explicit", opts: Options{ExplicitPath: "/tmp/store.db"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresProject(tt.opts); got != tt.want {
				t.Fatalf("RequiresProject = %v, want %v", got, tt.want)
			}
		})
	}
}
