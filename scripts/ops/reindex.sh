#!/usr/bin/env bash
# Rebuild search projection. Usage: SHARED_MEMORY_ENV=.env bash scripts/ops/reindex.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
export SHARED_MEMORY_ENV="${SHARED_MEMORY_ENV:-.env}"
echo "Running worker -reindex ..."
go run ./cmd/worker -reindex
