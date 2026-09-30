package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cosgroveb/guhd/internal/config"
)

func TestRunFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, tt := range []struct {
		args   []string
		code   int
		output string
	}{
		{[]string{"--help"}, 0, "Usage: guhd"},
		{[]string{"-h"}, 0, "Usage: guhd"},
		{[]string{"--version"}, 0, "guhd dev\n"},
		{[]string{"--unknown"}, 2, ""},
		{[]string{"--config"}, 2, ""},
		{[]string{"--config", ""}, 2, ""},
		{[]string{"unexpected"}, 2, ""},
		{nil, 1, ""},
	} {
		var stdout, stderr bytes.Buffer
		got := run(tt.args, os.Stdin, &stdout, &stderr)
		if got != tt.code {
			t.Fatalf("%v exit %d, want %d: %s", tt.args, got, tt.code, &stderr)
		}
		if tt.code == 0 {
			if !strings.Contains(stdout.String(), tt.output) || stderr.Len() != 0 {
				t.Fatalf("%v stdout=%q stderr=%q", tt.args, &stdout, &stderr)
			}
		} else if stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("%v stdout=%q stderr=%q", tt.args, &stdout, &stderr)
		}
	}
}

func TestStartupConfigMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	for _, tt := range []struct{ explicit, setup, wantError bool }{{false, false, false}, {true, false, true}, {true, true, false}, {false, true, false}} {
		cfg, setup, err := startupConfig(path, tt.explicit, tt.setup)
		if (err != nil) != tt.wantError {
			t.Fatalf("%+v: %v", tt, err)
		}
		if err == nil && (!setup || cfg.RefreshSeconds != 300) {
			t.Fatalf("missing config: %+v, setup=%t", cfg, setup)
		}
	}
}

func TestStartupConfigExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Defaults()
	cfg.Account = "person@example.com"
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	for _, setup := range []bool{false, true} {
		got, needsSetup, err := startupConfig(path, true, setup)
		if err != nil || needsSetup != setup || got.Account != cfg.Account {
			t.Fatalf("got %+v, setup %t, error %v", got, needsSetup, err)
		}
	}
	if err := os.WriteFile(path, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := startupConfig(path, false, true); err == nil {
		t.Fatal("setup ignored malformed config")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestRunOutputFailure(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		var stderr bytes.Buffer
		if code := run([]string{arg}, os.Stdin, failingWriter{}, &stderr); code != 1 {
			t.Fatalf("%s exit %d, want 1", arg, code)
		}
		if !strings.Contains(stderr.String(), "write failed") {
			t.Fatalf("%s stderr=%q", arg, &stderr)
		}
	}
}
