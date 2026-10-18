#!/bin/sh
#
# Builds a local image and runs it for quick manual testing in a browser.
# Single-platform `docker build` (fast), not Build-QuizWeb.sh's multi-arch
# buildx + registry push. Uses its own container name/tag so it won't
# clobber a real Start-QuizWeb.sh instance running alongside it.
#
# Usage:
#   ./Dev-QuizWeb.sh
#   ./Dev-QuizWeb.sh --port 9090

set -e

TAG='99quiz:local'
NAME='99quiz-dev'
PORT=8080
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT_DIR=$(dirname "$SCRIPT_DIR")
STATE_DIR="$SCRIPT_DIR/quiz-state"

while [ $# -gt 0 ]; do
    case "$1" in
        --port) PORT="$2"; shift 2 ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

cd "$ROOT_DIR"
docker build -f web/Dockerfile -t "$TAG" .

mkdir -p "$STATE_DIR"
chmod 0777 "$STATE_DIR"

docker rm -f "$NAME" >/dev/null 2>&1 || true
docker run --rm -d -p "${PORT}:8080" --name "$NAME" -v "${STATE_DIR}:/data/state" "$TAG"

echo "Running on http://localhost:${PORT} (container: $NAME)"
