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

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/quizweb .

FROM scratch
COPY --from=build /out/quizweb /quizweb
EXPOSE 8080
USER 65532:65532
# Swap state (working/pool.json schedule) is persisted here - mount a volume
# so it survives a restart/redeploy, e.g. docker run -v quizstate:/data/state.
VOLUME ["/data/state"]
ENTRYPOINT ["/quizweb"]
