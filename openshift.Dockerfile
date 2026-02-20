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

FROM registry.redhat.io/openshift-gitops-1/argo-rollouts-rhel8@sha256:719e2cf3d5046ab03c75673f3d43676b685d0af4b1cb6f03e11491177540d4a8
COPY --from=builder /workspace/manager /home/argo-rollouts/rollouts-plugin-metric-ai
# USER 65532:65532

# ENTRYPOINT ["/home/argo-rollouts/rollouts-plugin-metric-ai"]
