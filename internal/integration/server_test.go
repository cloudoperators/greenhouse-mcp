// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration_test

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"

	greenhousemcp "github.com/cloudoperators/greenhouse-mcp/internal/mcp/greenhouse"
	"github.com/cloudoperators/greenhouse-mcp/internal/utils"
)

const (
	mcpPath     = "/mcp"
	serviceName = "greenhouse-mcp-integration"
)

// startMCPServer wires up the real MCP server using the envtest REST config,
// starts it on a random port, and returns the full URL to the /mcp endpoint.
func startMCPServer(logger logr.Logger, restConfig *rest.Config) (string, error) {
	mcpServer := mcp.NewServer(&mcp.Implementation{
		Name:    serviceName,
		Version: "integration",
	}, nil)

	mcpServer.AddReceivingMiddleware(utils.CreateLoggingMiddleware(logger))

	if err := greenhousemcp.RegisterToolsWithConfig(logger, mcpServer, restConfig); err != nil {
		return "", fmt.Errorf("registering tools: %w", err)
	}

	handler := mcp.NewStreamableHTTPHandler(
		func(request *http.Request) *mcp.Server { return mcpServer },
		&mcp.StreamableHTTPOptions{
			DisableLocalhostProtection: true,
			Stateless:                  true,
		},
	)

	serveMux := http.NewServeMux()
	serveMux.Handle(mcpPath, utils.InjectHTTPMeta(handler))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("creating listener: %w", err)
	}

	httpServer := &http.Server{
		Handler:           serveMux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error(serveErr, "MCP server exited unexpectedly")
		}
	}()

	return fmt.Sprintf("http://%s%s", listener.Addr().String(), mcpPath), nil
}
