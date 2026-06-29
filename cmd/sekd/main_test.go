package main

import "testing"

func TestValidateProjectDirRejectsRoot(t *testing.T) {
	err := validateProjectDir("/")
	if err == nil {
		t.Fatal("expected root project dir to be rejected")
	}
}

func TestValidateProjectDirAllowsGlobalStore(t *testing.T) {
	if err := validateProjectDir("_global"); err != nil {
		t.Fatalf("expected _global to be allowed: %v", err)
	}
}

func TestValidateProjectDirAllowsRegularPath(t *testing.T) {
	if err := validateProjectDir("/tmp/sek-project"); err != nil {
		t.Fatalf("expected regular path to be allowed: %v", err)
	}
}
