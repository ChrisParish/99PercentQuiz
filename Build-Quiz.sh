#!/bin/sh
#
# Merges the per-category question files into the working pool and the
# delivered file. Thin wrapper around Build-Quiz.py.
#
# Usage:
#   ./Build-Quiz.sh
#   ./Build-Quiz.sh --seed 42

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
exec python3 "$SCRIPT_DIR/Build-Quiz.py" "$@"
