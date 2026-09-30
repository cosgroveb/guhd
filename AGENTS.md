# guhd

## Code

Keep one Go module and a thin `cmd/guhd` entrypoint. `internal/config` owns persisted settings, `internal/source` owns gog and Git subprocesses, and `internal/dashboard` owns terminal state and rendering.

Match surrounding code before broader conventions. Keep source results concrete, define small interfaces at consumers, and preserve gog's ownership of credentials.

## Checks

Run `make _fmt`, `make check`, and `go test -race ./...`. Checks require Go 1.26 or later, golangci-lint built with a compatible Go version, and pandoc.

Use fictional gog responses and temporary Git repositories in tests. Keep live Google data out of fixtures and subprocess output out of terminal rendering until sanitized.

## Changes

Keep changes focused and document user-visible behavior in `CHANGELOG.md`. Keep planning artifacts outside the repository.

Keep the command and configuration reference in `doc/guhd.1.md`.
