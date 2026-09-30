VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY := guhd
PREFIX ?= /usr/local
PANDOC ?= pandoc
DOC_MAN_DIR := build/docs/man
DOC_MAN_OUTPUTS := $(DOC_MAN_DIR)/guhd.1
LDFLAGS := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := build
.PHONY: build deb clean install run check test man help _fmt _fmt-check _vet _lint _check-docs _workflow-make-targets _release-check _require-pandoc _hooks

build: ## Build binary
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/guhd/

clean: ## Remove build outputs
	rm -f $(BINARY)
	rm -rf build/

install: build ## Install binary under PREFIX
	install -d "$(DESTDIR)$(PREFIX)/bin"
	install -m 0755 "$(BINARY)" "$(DESTDIR)$(PREFIX)/bin/$(BINARY)"

run: ## Run binary
	go run ./cmd/guhd/

check: _fmt-check _vet _lint _workflow-make-targets _release-check _check-docs test build

deb: ## Build Debian binary and source packages in dist
	INCLUDE_SOURCE=1 scripts/build-deb "$(VERSION)"

_release-check:
	actionlint
	scripts/test-release-helpers

test: ## Run tests
	go test ./...

man: $(DOC_MAN_OUTPUTS) ## Generate man pages from markdown

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "; printf "Available targets:\n"} \
		/^##@/ {printf "\n%s\n", substr($$0, 5)} \
		/^[a-zA-Z0-9][a-zA-Z0-9_-]*:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

_fmt:
	go fmt ./...

_fmt-check:
	@files=$$(find . -type f -name '*.go' -not -path './.git/*'); \
	unformatted=$$(gofmt -l $$files); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt: these files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

_vet:
	go vet ./...

_lint:
	golangci-lint run

_check-docs: man

_workflow-make-targets:
	./scripts/check-workflow-make-targets.sh

_require-pandoc:
	@command -v "$(PANDOC)" >/dev/null 2>&1 || { \
		echo "error: pandoc is required for docs generation" >&2; \
		exit 1; \
	}

_hooks:
	git config core.hooksPath .githooks

$(DOC_MAN_DIR)/%.1: doc/%.1.md | _require-pandoc
	mkdir -p "$(DOC_MAN_DIR)"
	"$(PANDOC)" --from=markdown-smart --to=man --standalone "$<" -o "$@"
