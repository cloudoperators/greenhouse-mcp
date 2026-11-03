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

FROM alpine:3.23.3 AS tools
RUN apk --no-cache add curl \
  && curl -fsSL https://dl.k8s.io/release/v1.35.4/bin/linux/amd64/kubectl \
     -o /usr/local/bin/kubectl \
  && chmod +x /usr/local/bin/kubectl \
  && curl -fsSL https://github.com/openfga/cli/releases/download/v0.7.17/fga_0.7.17_linux_amd64.tar.gz \
     | tar -xz -C /usr/local/bin fga \
  && chmod +x /usr/local/bin/fga

FROM alpine:3.23.3
RUN apk --no-cache add ca-certificates curl jq \
  && update-ca-certificates
WORKDIR /
COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /workspace/bin/* .
COPY --from=tools /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY --from=tools /usr/local/bin/fga /usr/local/bin/fga
USER 65534:65534

ENTRYPOINT ["/mcp-greenhouse"]
