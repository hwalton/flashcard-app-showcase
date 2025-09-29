#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${SCRIPT_DIR}/../control-panel-app-flashcards/cmd/control-panel"

pushd "$APP_DIR" >/dev/null
go run main.go assign-all-cards --prod -- info
popd >/dev/null