#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
make build
go test -run '^$' -bench . -benchmem ./internal/... ./cmd/fdpi
if [[ -x /usr/bin/time ]]; then
    for trial in 1 2 3 4 5; do
        /usr/bin/time -f 'CLI elapsed=%e s maxRSS=%M KiB' ./build/fdpi --version
    done
fi
printf '%s\n' 'TUI render: BenchmarkInitialView; actual startup + maxRSS: BenchmarkTUIStartupPTY.'
