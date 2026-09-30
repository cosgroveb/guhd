#!/usr/bin/env bash
# checks workflow YAML for make <target> references and fails if the target is not defined in Makefile.

set -euo pipefail

if [ "$#" -ne 0 ]; then
    echo "usage: $0" >&2
    exit 2
fi

targets_file=$(mktemp)
refs_file=$(mktemp)
trap 'rm -f "$targets_file" "$refs_file"' EXIT

(make -qp 2>/dev/null || true) \
    | awk -F: '/^[A-Za-z0-9][A-Za-z0-9_.-]*:([^=]|$)/ { print $1 }' \
    | sort -u >"$targets_file"

if compgen -G ".github/workflows/*.yml" >/dev/null || compgen -G ".github/workflows/*.yaml" >/dev/null; then
    grep -RhoE 'make[[:space:]]+[A-Za-z0-9_.-]+' .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null \
        | awk '{ print $2 }' \
        | sort -u >"$refs_file" || true
fi

while IFS= read -r target; do
    if [ -z "$target" ]; then
        continue
    fi
    if ! grep -Fxq "$target" "$targets_file"; then
        echo "workflow references unknown make target: $target" >&2
        exit 1
    fi
done <"$refs_file"

echo "workflow make target check passed"
