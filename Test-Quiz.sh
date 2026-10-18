#!/bin/sh
#
# Deterministic validator for The 99% Quiz question set. Thin wrapper around
# Test-Quiz.py, which carries the actual rule engine.
#
# Usage:
#   ./Test-Quiz.sh --path working/pool.json
#   ./Test-Quiz.sh --path questions.json --strict
#   ./Test-Quiz.sh --path working/pool.json --fact-check-path working/factcheck/merged.json

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
exec python3 "$SCRIPT_DIR/Test-Quiz.py" "$@"
