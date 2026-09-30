# guhd

## Code

Keep `cmd/guhd` limited to startup and argument handling. `internal/config` stores settings, `internal/source` runs gog and Git, and `internal/dashboard` handles terminal state and rendering.

Match surrounding code, then file, package, project, and Go conventions. Keep interfaces small and preserve gog's ownership of credentials. Sanitize external text before terminal rendering.

## Checks

Run `make _fmt`, `make check`, and `go test -race ./...` for code changes. Checks require Go 1.26 or later, golangci-lint built with a compatible Go version, and pandoc.

Use fictional gog responses and temporary Git repositories in tests. Keep live Google data out of fixtures.

## Changes

Keep changes focused and record user-visible behavior in `CHANGELOG.md`. Keep planning artifacts outside the repository.

Maintain the command and configuration reference in `doc/guhd.1.md`. Run `make man` after editing it.
