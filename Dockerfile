# Build the manager binary
FROM golang:1.25@sha256:8305f5fa8ea63c7b5bc85bd223ccc62941f852318ebfbd22f53bbd0b358c07e1 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY internal/ internal/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager main.go

ARG ROLLOUTS_VERSION=v1.9.0
ARG ROLLOUTS_DIGEST=sha256:13346024500167f71815e1dcd7671f46378137e726ef03b2baf69a188224f813
FROM quay.io/argoproj/argo-rollouts:${ROLLOUTS_VERSION}@${ROLLOUTS_DIGEST}
COPY --from=builder /workspace/manager /home/argo-rollouts/rollouts-plugin-metric-ai
# USER 65532:65532

# ENTRYPOINT ["/home/argo-rollouts/rollouts-plugin-metric-ai"]
