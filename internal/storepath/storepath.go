package storepath

import (
	"errors"
	"os"
	"path/filepath"
)

type Options struct {
	ProjectDir   string
	DataDir      string
	ExplicitPath string
	Global       bool
}

func Resolve(opts Options) (string, error) {
	if opts.ExplicitPath != "" {
		return opts.ExplicitPath, nil
	}
	if opts.Global {
		dataDir := opts.DataDir
		if dataDir == "" {
			var err error
			dataDir, err = DefaultDataDir()
			if err != nil {
				return "", err
			}
		}
		return filepath.Join(dataDir, "store.db"), nil
	}
	if opts.ProjectDir == "" {
		return "", errors.New("project directory is required for project-local store")
	}
	return filepath.Join(opts.ProjectDir, ".sek", "store.db"), nil
}

func RequiresProject(opts Options) bool {
	return !opts.Global && opts.ExplicitPath == ""
}

func DefaultDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sek"), nil
}
