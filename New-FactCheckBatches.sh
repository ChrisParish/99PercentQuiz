#!/bin/sh
#
# Emits blind fact-check batches for a second model to answer. Thin wrapper
# around New-FactCheckBatches.py.
#
# Usage:
#   ./New-FactCheckBatches.sh
#   ./New-FactCheckBatches.sh --batch-size 25

set -e

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
exec python3 "$SCRIPT_DIR/New-FactCheckBatches.py" "$@"
