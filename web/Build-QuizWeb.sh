#!/bin/sh
#
# Builds the quiz web container image for linux/amd64 and linux/arm64
# (e.g. for a Raspberry Pi).
#
# The build context is the 99-quiz directory so that questions.json is in
# scope. go vet and go test run inside the builder stage, so a failing test
# means no image is produced.
#
# Uses `docker buildx` so it can be built on Apple Silicon (arm64) and still
# run on typical amd64 cloud hosts as well as arm64 boards like a Raspberry
# Pi. Requires Docker Desktop (or another buildx-enabled setup) with QEMU
# emulation support for cross-platform builds.
#
# `docker load` only understands single-platform images, so a true
# multi-arch manifest can't be loaded locally in one go. Instead this builds
# each platform separately with --load, tagging each with an arch suffix so
# both are available locally at once. To publish a single multi-arch tag
# that a registry/Pi can pull transparently, push a manifest list built from
# these two images (see the printed hint at the end).
#
# Usage:
#   ./Build-QuizWeb.sh
#   ./Build-QuizWeb.sh 99quiz:2026-09-02

set -e

REGISTRY='registry.cjparish.uk'
VERSION="${1:-99quiz:latest}"
TAG="${REGISTRY}/${VERSION}"
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
ROOT_DIR=$(dirname "$SCRIPT_DIR")
PLATFORMS="linux/amd64 linux/arm64"

cd "$ROOT_DIR"

for PLATFORM in $PLATFORMS; do
    ARCH=$(echo "$PLATFORM" | cut -d/ -f2)
    ARCH_TAG="${TAG}-${ARCH}"

    echo "Building $ARCH_TAG for $PLATFORM..."
    if ! docker buildx build --platform "$PLATFORM" -f web/Dockerfile \
        --build-arg VERSION="$VERSION" -t "$ARCH_TAG" --load .; then
        echo 'BUILD FAILED' >&2
        exit 1
    fi

    SIZE=$(docker image inspect "$ARCH_TAG" --format '{{.Size}}')
    SIZE_MB=$(awk "BEGIN { printf \"%.1f\", $SIZE / 1048576 }")
    echo "Built $ARCH_TAG (${SIZE_MB} MB) for $PLATFORM"
done

echo
echo "To publish a single multi-arch tag '$TAG', push both images then create a manifest list, e.g.:"
docker push ${TAG}-amd64
docker push ${TAG}-arm64
docker manifest create ${TAG} ${TAG}-amd64 ${TAG}-arm64
docker manifest push ${TAG}
