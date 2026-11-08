#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
export FDPI_TEST_ENGINE="${FDPI_TEST_ENGINE:-$PWD/build/nfqws2}"
[[ -x $FDPI_TEST_ENGINE ]] || { echo 'Сначала выполните make engine или задайте FDPI_TEST_ENGINE с путём к настоящему nfqws' >&2; exit 1; }
export FDPI_HOST_NETNS
FDPI_HOST_NETNS=$(readlink /proc/self/ns/net)
export FDPI_NETNS_TEST=1
# map-auto allows setgroups required by upstream nfqws. Requires newuidmap/subuid.
unshare -Urn --map-auto go test -p 1 -count=1 -v ./internal/network ./internal/service
