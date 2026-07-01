package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/seryogakovalyov/sek/internal/store"
)

func TestManagerRecordsSessionSnapshots(t *testing.T) {
	db := filepath.Join(t.TempDir(), "store.db")
	st, err := store.NewSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	projectDir := t.TempDir()
	manager := NewManager(Options{
		Store:      st,
		SessionID:  "sek-test",
		ProjectDir: projectDir,
	})

	ctx := context.Background()
	manager.Start(ctx)
	manager.Finish(ctx)

	session, err := st.GetSession(ctx, "sek-test")
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "finished" {
		t.Fatalf("expected finished status, got %q", session.Status)
	}
	if session.ProjectDir != projectDir {
		t.Fatalf("expected project dir %q, got %q", projectDir, session.ProjectDir)
	}
	if session.StartSnapshot == nil || session.EndSnapshot == nil {
		t.Fatalf("expected start and end snapshots: %#v", session)
	}
	if session.StartSnapshot.Available || session.EndSnapshot.Available {
		t.Fatalf("non-git temp dir should not produce available snapshots")
	}
}

func TestManagerRecoversOpenSessions(t *testing.T) {
	db := filepath.Join(t.TempDir(), "store.db")
	st, err := store.NewSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	projectDir := t.TempDir()
	ctx := context.Background()
	first := NewManager(Options{
		Store:      st,
		SessionID:  "sek-open",
		ProjectDir: projectDir,
	})
	first.Start(ctx)

	second := NewManager(Options{
		Store:      st,
		SessionID:  "sek-next",
		ProjectDir: projectDir,
	})
	second.Start(ctx)

	recovered, err := st.GetSession(ctx, "sek-open")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "interrupted" {
		t.Fatalf("expected interrupted status, got %q", recovered.Status)
	}
	if recovered.EndSnapshot == nil {
		t.Fatalf("expected recovery end snapshot")
	}

	current, err := st.GetSession(ctx, "sek-next")
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "running" {
		t.Fatalf("expected current session to remain running, got %q", current.Status)
	}
}

func TestCaptureGitSnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot := CaptureGitSnapshot(context.Background(), dir)
	if !snapshot.Available {
		t.Fatalf("expected git snapshot: %#v", snapshot)
	}
	if !snapshot.Dirty {
		t.Fatalf("expected dirty snapshot: %#v", snapshot)
	}
	if len(snapshot.ChangedFiles) != 1 || snapshot.ChangedFiles[0] != "README.md" {
		t.Fatalf("unexpected changed files: %#v", snapshot.ChangedFiles)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
