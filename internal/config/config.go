// Package config owns the dashboard's persisted settings.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Theme          string   `json:"theme"`
	Account        string   `json:"account"`
	Client         string   `json:"client"`
	Calendars      []string `json:"calendars"`
	MailQuery      string   `json:"mail_query"`
	Projects       []string `json:"projects"`
	RefreshSeconds int      `json:"refresh_seconds"`
}

func Defaults() Config {
	return Config{Theme: "plain", Client: "default", MailQuery: "in:inbox", RefreshSeconds: 300}
}

func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "guhd", "config.json"), nil
}

func Validate(cfg Config) error {
	for _, value := range append(append([]string{cfg.Account, cfg.Client, cfg.MailQuery}, cfg.Calendars...), cfg.Projects...) {
		if strings.ContainsRune(value, 0) {
			return errors.New("configuration values must not contain NUL characters")
		}
	}
	address, err := mail.ParseAddress(cfg.Account)
	if err != nil || address.Address != cfg.Account {
		return errors.New("account must be an email address")
	}
	if strings.TrimSpace(cfg.MailQuery) == "" {
		return errors.New("mail_query must not be empty")
	}
	if cfg.RefreshSeconds <= 0 || int64(cfg.RefreshSeconds) > int64((1<<63-1)/time.Second) {
		return errors.New("refresh_seconds must be positive and fit a time duration")
	}
	for _, id := range cfg.Calendars {
		if strings.TrimSpace(id) == "" {
			return errors.New("calendars must not contain empty IDs")
		}
	}
	for _, path := range cfg.Projects {
		if !filepath.IsAbs(path) {
			return errors.New("projects must contain absolute directory paths")
		}
	}
	return nil
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected trailing JSON")
		}
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("validate config %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".guhd-*")
	if err != nil {
		return fmt.Errorf("create config %q: %w", path, err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("write config %q: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync config %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close config %q: %w", path, err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replace config %q: %w", path, err)
	}
	return nil
}
