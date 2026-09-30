package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func validConfig() Config {
	c := Defaults()
	c.Account = "person@example.com"
	c.Calendars = []string{"Primary-ID"}
	return c
}

func TestSaveLoadAndReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guhd", "config.json")
	cfg := validConfig()
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file permissions %o", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory permissions %o", dir.Mode().Perm())
	}
	old, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = old.Close() }()
	cfg.MailQuery = "is:unread"
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("got %+v, want %+v", got, cfg)
	}
	oldInfo, err := old.Stat()
	if err != nil {
		t.Fatal(err)
	}
	newInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldInfo, newInfo) {
		t.Fatal("save overwrote existing inode")
	}
	cfg.RefreshSeconds = 0
	if err := Save(path, cfg); err == nil {
		t.Fatal("invalid save succeeded")
	}
	got, err = Load(path)
	if err != nil || got.RefreshSeconds != 300 {
		t.Fatalf("invalid save changed file: %+v, %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary files left: %v", entries)
	}
}

func TestLoadStrictJSON(t *testing.T) {
	for _, body := range []string{`{`, `{"account":"person@example.com","unknown":1}`, `{"account":"person@example.com"} {}`, `{"account":"person@example.com"} trailing`, `null`, `{"account":"invalid"}`, `{"account":"person@example.com","refresh_seconds":0}`} {
		t.Run(body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("want path error, got %v", err)
			}
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"account":"person@example.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.RefreshSeconds != 300 || got.MailQuery != "in:inbox" || got.Client != "default" {
		t.Fatalf("defaults missing: %+v", got)
	}
}

func TestValidate(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Account = "Name <person@example.com>" },
		func(c *Config) { c.Client = "bad\x00client" },
		func(c *Config) { c.MailQuery = " " },
		func(c *Config) { c.Calendars = []string{""} },
		func(c *Config) { c.Projects = []string{"relative"} },
		func(c *Config) { c.RefreshSeconds = -1 },
	} {
		cfg := validConfig()
		change(&cfg)
		if err := Validate(cfg); err == nil {
			t.Fatalf("accepted invalid config: %+v", cfg)
		}
	}
	cfg := validConfig()
	cfg.Calendars = nil
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "guhd", "config.json"); got != want {
		t.Fatalf("path %q, want %q", got, want)
	}
}
