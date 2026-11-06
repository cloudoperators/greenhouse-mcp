// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/cloudoperators/greenhouse-mcp/internal/mcp/greenhouse"
	"github.com/cloudoperators/greenhouse-mcp/internal/tracing"
	"github.com/cloudoperators/greenhouse-mcp/internal/utils"
)

const (
	port        = 8085
	mcpPath     = "/mcp"
	healthzPath = "/healthz"
	serviceName = "greenhouse-mcp"

	flagGreenhouseAPIServer = "greenhouse-api-server"
	flagBindAddress         = "app-bind-address"
	flagCAURL               = "ca-url"
	flagCAFile              = "ca-file"
	flagOTelEndpoint        = "otel-endpoint"

	defaultBindAddress         = "localhost"
	defaultGreenhouseAPIServer = "https://greenhouse.example.com"
	defaultCAURL               = "https://greenhouse.example.com/clusters/cluster-ca"
)

var (
	serverVer           = "dev"
	gitCommit           = "unknown"
	greenhouseAPIServer string
	bindAddress         string
	caURL               string
	caFile              string
	otelEndpoint        string
)

func init() {
	flag.StringVar(&greenhouseAPIServer, flagGreenhouseAPIServer, defaultGreenhouseAPIServer, "URL of the Greenhouse API server")
	flag.StringVar(&bindAddress, flagBindAddress, defaultBindAddress, "Address to bind the HTTP server to (default: localhost)")
	flag.StringVar(&caURL, flagCAURL, defaultCAURL, "URL to fetch the Greenhouse cluster CA certificate from")
	flag.StringVar(&caFile, flagCAFile, "", "Path to a PEM-encoded CA certificate file (takes precedence over --ca-url)")
	flag.StringVar(&otelEndpoint, flagOTelEndpoint, "", "OTLP gRPC endpoint for tracing (empty = disabled)")
}

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := ctrl.Log

	caData, err := utils.FetchCA(context.Background(), caURL, caFile)
	if err != nil {
		log.Fatalf("failed to load CA: %v", err)
	}

	shutdown, err := tracing.Init(context.Background(), serviceName, serverVer, otelEndpoint)
	if err != nil {
		log.Fatalf("failed to init tracing: %v", err)
	}
	defer func() { shutdown(context.Background()) }() //nolint:errcheck

	mux := http.NewServeMux()
	mux.HandleFunc(healthzPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    serviceName,
		Version: serverVer,
	}, nil)

	server.AddReceivingMiddleware(tracing.Middleware(logger, serviceName))
	server.AddReceivingMiddleware(utils.CreateLoggingMiddleware(logger))

	if err := greenhouse.RegisterTools(logger, server, greenhouseAPIServer, caData); err != nil {
		log.Fatalf("failed to register tools: %v", err) //nolint:gocritic // startup failure; tracing shutdown is best-effort
	}

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
		Stateless:                  true,
	})

	mux.Handle(mcpPath, utils.InjectHTTPMeta(handler))

	listenAddr := net.JoinHostPort(bindAddress, strconv.Itoa(port))
	figure.NewColorFigure("Greenhouse MCP", "doom", "green", true).Print()
	figure.NewColorFigure("MCP Server Version: "+serverVer+" ("+gitCommit+")", "term", "yellow", true).Print()
	fmt.Println()
	logger.Info("starting server", "address", listenAddr)
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}
