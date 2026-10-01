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
	t.Chdir(t.TempDir())
	cfg := config.Defaults()
	cfg.Account = "person@example.com"
	for _, path := range []string{"work.json", "-setup", "--version", "--", "-help", "-test.v", "-htest.v"} {
		if err := config.Save(path, cfg); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		args   []string
		code   int
		output string
		err    string
	}{
		{[]string{"--help"}, 0, "Usage: guhd", ""},
		{[]string{"-h"}, 0, "Usage: guhd", ""},
		{[]string{"--version"}, 0, "guhd dev\n", ""},
		{[]string{"--config", "work.json"}, 1, "", "interactive terminal"},
		{[]string{"--config=work.json"}, 1, "", "interactive terminal"},
		{[]string{"--setup"}, 1, "", "interactive terminal"},
		{[]string{"--config", "-setup"}, 1, "", "interactive terminal"},
		{[]string{"--config=-setup"}, 1, "", "interactive terminal"},
		{[]string{"--config", "--version"}, 1, "", "interactive terminal"},
		{[]string{"--config", "--"}, 1, "", "interactive terminal"},
		{[]string{"--config", "-help"}, 1, "", "interactive terminal"},
		{[]string{"-setup"}, 2, "", "use --setup instead of -setup"},
		{[]string{"-setup=true"}, 2, "", "use --setup instead of -setup"},
		{[]string{"-config", "work.json"}, 2, "", "use --config instead of -config"},
		{[]string{"-config=work.json"}, 2, "", "use --config instead of -config"},
		{[]string{"-version"}, 2, "", "use --version instead of -version"},
		{[]string{"-version=true"}, 2, "", "use --version instead of -version"},
		{[]string{"-help"}, 2, "", "use --help instead of -help"},
		{[]string{"-help=true"}, 2, "", "use --help instead of -help"},
		{[]string{"-htest.v"}, 2, "", "unknown flag: -htest.v"},
		{[]string{"-hhtest.v"}, 2, "", "unknown flag: -hhtest.v"},
		{[]string{"-htest.v=true"}, 2, "", "unknown flag: -htest.v=true"},
		{[]string{"--config", "-htest.v"}, 1, "", "interactive terminal"},
		{[]string{"--config=-htest.v"}, 1, "", "interactive terminal"},
		{[]string{"-h=true"}, 0, "Usage: guhd", ""},
		{[]string{"-h=false"}, 1, "", "interactive terminal"},
		{[]string{"-h=test.v"}, 2, "", "invalid argument"},
		{[]string{"-test.v"}, 2, "", "unknown flag: -test.v"},
		{[]string{"-test.v=true"}, 2, "", "unknown flag: -test.v=true"},
		{[]string{"--version", "-test.v"}, 2, "", "unknown flag: -test.v"},
		{[]string{"--config", "-test.v"}, 1, "", "interactive terminal"},
		{[]string{"--config=-test.v"}, 1, "", "interactive terminal"},
		{[]string{"--", "-test.v"}, 2, "", "unexpected positional arguments"},
		{[]string{"unexpected", "-test.v"}, 2, "", "unexpected positional arguments"},
		{[]string{"-elp"}, 2, "", "unknown shorthand flag: 'e' in -elp"},
		{[]string{"-h", "-elp"}, 2, "", "unknown shorthand flag: 'e' in -elp"},
		{[]string{"--unknown", "-setup"}, 2, "", "unknown flag: --unknown"},
		{[]string{"--setup=invalid", "-test.v"}, 2, "", "invalid argument"},
		{[]string{"--unknown"}, 2, "", "unknown flag: --unknown"},
		{[]string{"-x"}, 2, "", "unknown shorthand flag"},
		{[]string{"--config"}, 2, "", "flag needs an argument: --config"},
		{[]string{"--config", ""}, 2, "", "--config requires a nonempty path"},
		{[]string{"--config="}, 2, "", "--config requires a nonempty path"},
		{[]string{"--setup=invalid"}, 2, "", "invalid argument"},
		{[]string{"unexpected"}, 2, "", "unexpected positional arguments"},
		{[]string{"--", "-setup"}, 2, "", "unexpected positional arguments"},
		{[]string{"--help", "unexpected"}, 2, "", "unexpected positional arguments"},
		{[]string{"unexpected", "--help"}, 2, "", "unexpected positional arguments"},
		{[]string{"--help", "--unknown"}, 2, "", "unknown flag"},
		{[]string{"--"}, 1, "", "interactive terminal"},
		{nil, 1, "", "interactive terminal"},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(tt.args, os.Stdin, &stdout, &stderr)
			if got != tt.code {
				t.Fatalf("%v exit %d, want %d: %s", tt.args, got, tt.code, &stderr)
			}
			if tt.code == 0 {
				if !strings.Contains(stdout.String(), tt.output) || stderr.Len() != 0 {
					t.Fatalf("%v stdout=%q stderr=%q", tt.args, &stdout, &stderr)
				}
			} else if stdout.Len() != 0 || !strings.Contains(stderr.String(), tt.err) {
				t.Fatalf("%v stdout=%q stderr=%q, want error %q", tt.args, &stdout, &stderr, tt.err)
			}
		})
	}
}

func TestRunHelpWithoutRuntime(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("PATH", "")
	for _, arg := range []string{"--help", "-h", "--version"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{arg}, nil, &stdout, &stderr); code != 0 {
			t.Fatalf("%s exit %d: %s", arg, code, &stderr)
		}
		if stderr.Len() != 0 || stdout.Len() == 0 {
			t.Fatalf("%s stdout=%q stderr=%q", arg, &stdout, &stderr)
		}
		if arg != "--version" {
			for _, text := range []string{"--setup", "--config PATH", "-h, --help", "--version", "Examples:", "guhd --setup", "guhd --config work.json"} {
				if !strings.Contains(stdout.String(), text) {
					t.Fatalf("help missing %q: %s", text, &stdout)
				}
			}
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
