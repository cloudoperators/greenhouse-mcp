<!--
  ~ SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
  ~ SPDX-License-Identifier: Apache-2.0
-->

Greenhouse MCP
==============
[![REUSE status](https://api.reuse.software/badge/github.com/cloudoperators/greenhouse-mcp)](https://api.reuse.software/info/github.com/cloudoperators/greenhouse-mcp)
<a href="https://github.com/cloudoperators/greenhouse"><img align="left" width="150" height="170" src="./docs/assets/greenhouse.svg"></a>

Greenhouse MCP is a [Model Context Protocol](https://modelcontextprotocol.io/) server for [Greenhouse](https://github.com/cloudoperators/greenhouse), the day 2 operations platform for managing cloud native infrastructure.

It exposes Greenhouse resources and operations as MCP tools, enabling AI assistants to inspect and reason about a fleet of Kubernetes clusters, plugins and plugin definitions managed by Greenhouse.

<br clear="left"/>

## Value Propositions

1. **Conversational operations** <br>
   Interact with Greenhouse-managed clusters, plugins and plugin definitions through an AI assistant using natural language.
2. **Debugging assistance** <br>
   Surface cluster and plugin state to help diagnose issues across a fleet of Kubernetes clusters.
3. **Standard protocol** <br>
   Built on the [Model Context Protocol](https://modelcontextprotocol.io/), so it integrates with any MCP-compatible client.
4. **Secure by design** <br>
   Scoped access to Greenhouse resources, aligned with the platform's fine-grained access control.

## Usage

The server exposes MCP over streamable HTTP. By default it listens on port `8085` with the MCP endpoint at `/mcp` and a health check at `/healthz`.

### Build & run

```sh
# build the binary to ./bin/mcp-greenhouse
make build

# or run directly from source
make run
```

Configuration is provided via flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--greenhouse-api-server` | `https://greenhouse.example.com` | URL of the Greenhouse API server. |
| `--ca-url` | `https://greenhouse.example.com/clusters/cluster-ca` | URL to fetch the Greenhouse cluster CA certificate from. Expected to return JSON with a `certs` field containing the PEM data. |
| `--app-bind-address` | `localhost` | Address to bind the HTTP server to. |
| `--otel-endpoint` | _(empty)_ | OTLP gRPC endpoint for tracing. Empty disables tracing. |

```sh
./bin/mcp-greenhouse \
  --greenhouse-api-server=https://greenhouse.example.com \
  --ca-url=https://greenhouse.example.com/clusters/cluster-ca
```

The server authenticates each request with the caller's bearer token via Kubernetes `TokenReview` + `SubjectAccessReview`, so access is scoped to what the caller is allowed to see in Greenhouse.

### Container image

```sh
make docker-build   # builds greenhouse-mcp:latest
```

## Community

Greenhouse MCP is a community-driven project. We welcome contributions and feedback. Please see our [Contributing Guidelines](CONTRIBUTING.md) for more information.

## Documentation

User guides, links and references for the Greenhouse platform are available in the official [Greenhouse documentation](https://cloudoperators.github.io/greenhouse/).

### Architecture & Design

The [Greenhouse design and architecture document](https://cloudoperators.github.io/greenhouse/docs/architecture/product_design/) describes the various use-cases and user stories of the platform this server integrates with.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2026 SAP SE or an SAP affiliate company and Greenhouse contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/cloudoperators/greenhouse-mcp).