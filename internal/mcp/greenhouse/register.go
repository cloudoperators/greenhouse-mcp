// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package greenhouse

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"

	"github.com/cloudoperators/greenhouse-mcp/internal/mcp/greenhouse/tools"
)

func RegisterTools(log logr.Logger, server *mcp.Server, apiServer string, caData []byte) error {
	greenhouse, err := tools.NewGreenhouse(log, apiServer, caData)
	if err != nil {
		return fmt.Errorf("initializing greenhouse client: %w", err)
	}

	registerToolHandlers(server, greenhouse)
	return nil
}

// RegisterToolsWithConfig wires up all tools using an already-resolved
// *rest.Config. Used by integration tests to inject an envtest config
// without going through in-cluster / kubeconfig discovery.
func RegisterToolsWithConfig(log logr.Logger, server *mcp.Server, cfg *rest.Config) error {
	greenhouse, err := tools.NewGreenhouseWithConfig(log, cfg)
	if err != nil {
		return fmt.Errorf("initializing greenhouse client from config: %w", err)
	}

	registerToolHandlers(server, greenhouse)
	return nil
}

func registerToolHandlers(server *mcp.Server, greenhouse *tools.Greenhouse) {
	mcp.AddTool(server, tools.ClusterTool(), greenhouse.GetCluster)
	mcp.AddTool(server, tools.ClusterSchemaTool(), greenhouse.GetClusterSchema)

	mcp.AddTool(server, tools.CatalogTool(), greenhouse.GetCatalog)
	mcp.AddTool(server, tools.CatalogSchemaTool(), greenhouse.GetCatalogSchema)

	mcp.AddTool(server, tools.PluginTool(), greenhouse.GetPlugin)
	mcp.AddTool(server, tools.ListPluginsTool(), greenhouse.ListPlugins)
	mcp.AddTool(server, tools.PluginSchemaTool(), greenhouse.GetPluginSchema)

	mcp.AddTool(server, tools.PluginDefinitionTool(), greenhouse.GetPluginDefinition)
	mcp.AddTool(server, tools.PluginDefinitionSchemaTool(), greenhouse.GetPluginDefinitionSchema)

	mcp.AddTool(server, tools.DebugPluginTool(), greenhouse.DebugPlugin)
	mcp.AddTool(server, tools.DebugClusterTool(), greenhouse.DebugCluster)

	server.AddPrompt(tools.DebugClusterPrompt(), tools.DebugClusterPromptHandler)
}
