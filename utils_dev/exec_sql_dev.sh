#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_DIR="${SCRIPT_DIR}/../control-panel-app-flashcards/cmd/control-panel"

# read ./input (next to this script) into a single shell variable
INPUT_FILE="${SCRIPT_DIR}/input.sql"
if [ -f "$INPUT_FILE" ]; then
  # read whole file preserving newlines into variable
  SQL_INPUT="$(<"$INPUT_FILE")"
else
  SQL_INPUT=""
fi

pushd "$APP_DIR" >/dev/null
# pass the input as a second positional argument; use -- to stop flag parsing
go run main.go exec-sql --dev -- "$SQL_INPUT"
popd >/dev/null