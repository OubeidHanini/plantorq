# One Dockerfile for every Go service: docker build --build-arg SERVICE=api .
# Stage 1 compiles a static binary; stage 2 ships it on distroless (no shell,
# no package manager, non-root): a ~20 MB image with a minimal attack surface.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG SERVICE
RUN test -n "$SERVICE" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE} \
    && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
# Writable data dir (edge-agent offline buffer); a named volume mounted here
# inherits this ownership, so the non-root user can write to it.
COPY --from=build --chown=65532:65532 /out/data /data
# Numeric UID (distroless "nonroot"): Kubernetes can only enforce
# runAsNonRoot when the image user is numeric.
USER 65532:65532
ENTRYPOINT ["/service"]
