package session

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/seryogakovalyov/sek/internal/models"
)

func CaptureGitSnapshot(ctx context.Context, projectDir string) *models.GitSnapshot {
	snapshot := &models.GitSnapshot{CapturedAt: time.Now()}

	root, err := gitOutput(ctx, projectDir, "rev-parse", "--show-toplevel")
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	snapshot.Available = true
	snapshot.RepoRoot = root

	head, err := gitOutput(ctx, projectDir, "rev-parse", "HEAD")
	if err == nil {
		snapshot.Head = head
	}

	status, err := gitOutput(ctx, projectDir, "status", "--porcelain")
	if err == nil {
		snapshot.Status = status
		snapshot.Dirty = status != ""
		snapshot.ChangedFiles = parseChangedFiles(status)
	}

	diffStat, err := gitOutput(ctx, projectDir, "diff", "--stat")
	if err == nil {
		snapshot.DiffStat = diffStat
	}

	return snapshot
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return "", err
		}
		return "", &gitError{err: err, output: text}
	}
	return strings.TrimSpace(string(out)), nil
}

type gitError struct {
	err    error
	output string
}

func (e *gitError) Error() string {
	return e.err.Error() + ": " + e.output
}

func parseChangedFiles(status string) []string {
	if status == "" {
		return nil
	}
	lines := strings.Split(status, "\n")
	files := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		path := ""
		if len(line) >= 4 && line[2] == ' ' {
			path = strings.TrimSpace(line[3:])
		} else {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				path = strings.Join(fields[1:], " ")
			}
		}
		if path == "" {
			continue
		}
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		if !seen[path] {
			files = append(files, path)
			seen[path] = true
		}
	}
	return files
}
