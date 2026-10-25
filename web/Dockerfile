# Build context is the 99-quiz directory, not web/, so that questions.json is reachable.
#   docker build -f web/Dockerfile -t 99quiz:latest .

FROM golang:1.23-alpine AS build
WORKDIR /src

COPY web/go.mod ./
COPY web/*.go ./
COPY web/templates ./templates
COPY web/static ./static

# go:embed cannot reach outside the package directory, so the pool is copied in
# rather than referenced where it lives. pool.json rather than questions.json
# because the rotation needs each question's category to keep days balanced.
COPY working/pool.json ./data/pool.json

# Tests gate the build: a failing test means no image is produced.
RUN go vet ./... && go test ./...

# Stamped into the binary so the running container can report which image it
# came from; the build scripts pass the tag they are about to apply.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" -o /out/quizweb .

# The runtime image has no shell to mkdir with, so the state directory is made
# here. A volume mounted over it inherits this ownership; without it Docker
# creates the path root-owned and the unprivileged runtime user cannot write.
RUN install -d -o 65532 -g 65532 /out/state

FROM scratch
COPY --from=build /out/quizweb /quizweb
COPY --from=build --chown=65532:65532 /out/state /data/state
EXPOSE 8080
USER 65532:65532
# Swap state is persisted here - mount a volume so it survives a restart,
# e.g. docker run -v quizstate:/data/state. A bind mount keeps the host's
# ownership, so chown it to 65532:65532 there.
VOLUME ["/data/state"]
ENTRYPOINT ["/quizweb"]
