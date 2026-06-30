package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunTopLevelHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "help"} {
		t.Run(arg, func(t *testing.T) {
			var stderr bytes.Buffer

			if code := run([]string{arg}, &stderr); code != 0 {
				t.Fatalf("run(%q) exit code = %d, want 0", arg, code)
			}

			output := stderr.String()
			if !strings.Contains(output, "Usage: sekctl <command> [flags]") {
				t.Fatalf("help output missing usage:\n%s", output)
			}
			if strings.Contains(output, "unknown command") {
				t.Fatalf("help output should not report unknown command:\n%s", output)
			}
		})
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stderr bytes.Buffer

	if code := run([]string{"nope"}, &stderr); code != 1 {
		t.Fatalf("run unknown command exit code = %d, want 1", code)
	}

	output := stderr.String()
	if !strings.Contains(output, "unknown command: nope") {
		t.Fatalf("unknown command output missing error:\n%s", output)
	}
	if !strings.Contains(output, "Usage: sekctl <command> [flags]") {
		t.Fatalf("unknown command output missing usage:\n%s", output)
	}
}

func TestSplitArgsKeepsFlagValues(t *testing.T) {
	task, flags := splitArgs([]string{
		"--llm-base-url", "http://localhost:8000/v1",
		"--max-entries", "5",
		"how", "to", "query",
	})

	if task != "how to query" {
		t.Fatalf("task = %q, want %q", task, "how to query")
	}

	want := []string{
		"--llm-base-url", "http://localhost:8000/v1",
		"--max-entries", "5",
	}
	if len(flags) != len(want) {
		t.Fatalf("flags = %#v, want %#v", flags, want)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Fatalf("flags = %#v, want %#v", flags, want)
		}
	}
}

func TestSplitArgsKeepsEqualsFlags(t *testing.T) {
	task, flags := splitArgs([]string{
		"--llm-base-url=http://localhost:8000/v1",
		"--trace",
		"how", "to", "query",
	})

	if task != "how to query" {
		t.Fatalf("task = %q, want %q", task, "how to query")
	}

	want := []string{
		"--llm-base-url=http://localhost:8000/v1",
		"--trace",
	}
	if len(flags) != len(want) {
		t.Fatalf("flags = %#v, want %#v", flags, want)
	}
	for i := range want {
		if flags[i] != want[i] {
			t.Fatalf("flags = %#v, want %#v", flags, want)
		}
	}
}
