#!/bin/sh
#
# Runs the quiz web container.
#
# Serves on the given host port. SSL is handled upstream by the reverse proxy,
# so the container speaks plain HTTP.
#
# Swap state (which question is currently shown in each day's slots) is
# persisted to STATE_DIR so it survives a restart. It's bind-mounted from the
# host and made world-writable, since the container runs as a non-root,
# unnamed UID (65532) and scratch images have no chown step to grant it
# ownership directly.
#
# Usage:
#   ./Start-QuizWeb.sh
#   ./Start-QuizWeb.sh --port 9090 --title 'Breakfast 99% Quiz'
#   ./Start-QuizWeb.sh --state-dir /var/lib/99quiz

set -e

TAG='99quiz:1.1'
PORT=8080
TITLE=''
DETACH=0
STATE_DIR="$(pwd)/quiz-state"

while [ $# -gt 0 ]; do
    case "$1" in
        --tag) TAG="$2"; shift 2 ;;
        --port) PORT="$2"; shift 2 ;;
        --title) TITLE="$2"; shift 2 ;;
        --state-dir) STATE_DIR="$2"; shift 2 ;;
        --detach) DETACH=1; shift ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

mkdir -p "$STATE_DIR"
chmod 0777 "$STATE_DIR"

set -- run --rm -p "${PORT}:8080" --name 99quiz -v "${STATE_DIR}:/data/state"
if [ "$DETACH" -eq 1 ]; then
    set -- "$@" -d
fi
if [ -n "$TITLE" ]; then
    set -- "$@" -e "QUIZ_TITLE=$TITLE"
fi
set -- "$@" "$TAG"

echo "Starting on http://localhost:${PORT} (state in ${STATE_DIR})"
docker "$@"
