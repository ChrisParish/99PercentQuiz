#!/bin/sh
#
# Runs gofmt, go vet and the Go test suite in a container.
#
# Go is not installed locally, so all toolchain work happens inside
# golang:1.23-alpine. A named volume caches the Go build cache between runs.
#
# Usage:
#   ./Test-QuizWeb.sh
#   ./Test-QuizWeb.sh --detailed

set -e

DETAILED=0
for arg in "$@"; do
    case "$arg" in
        --detailed) DETAILED=1 ;;
        *)
            echo "Unknown argument: $arg" >&2
            exit 1
            ;;
    esac
done

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT_DIR=$(dirname "$SCRIPT_DIR")

# go:embed cannot reach outside the package directory, so sync the pool in.
# pool.json rather than questions.json because the rotation needs categories.
mkdir -p "$SCRIPT_DIR/data"
cp "$ROOT_DIR/working/pool.json" "$SCRIPT_DIR/data/pool.json"

FMT=$(docker run --rm -v "$SCRIPT_DIR:/src" -w /src golang:1.23-alpine gofmt -l .)
if [ -n "$FMT" ]; then
    echo 'gofmt required on:' >&2
    echo "$FMT" | sed 's/^/  /' >&2
    exit 1
fi

if [ "$DETAILED" -eq 1 ]; then
    TEST_CMD='go vet ./... && go test ./... -v'
else
    TEST_CMD='go vet ./... && go test ./...'
fi

if ! docker run --rm \
    -v "$SCRIPT_DIR:/src" \
    -v 99quiz-gocache:/root/.cache/go-build \
    -w /src golang:1.23-alpine sh -c "$TEST_CMD"; then
    echo 'FAILED' >&2
    exit 1
fi

echo 'PASSED'
