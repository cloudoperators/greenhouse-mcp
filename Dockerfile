# Build the binaries
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.27.1 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ENV CGO_ENABLED=0

WORKDIR /workspace
COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
	--mount=type=cache,target=/root/.cache/go-build \
	make action-build CGO_ENABLED=${CGO_ENABLED} GOOS=${TARGETOS} GOARCH=${TARGETARCH} VERSION=${VERSION} GIT_COMMIT=${GIT_COMMIT}

FROM alpine:3.23.3
WORKDIR /
COPY --from=builder /workspace/bin/* .
USER 65534:65534

ENTRYPOINT ["/mcp-greenhouse"]
