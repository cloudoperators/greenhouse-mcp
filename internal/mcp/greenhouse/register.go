package greenhouse

import (
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cloudoperators/greenhouse-mcp/internal/mcp/greenhouse/tools"
)

func RegisterTools(log logr.Logger, server *mcp.Server, apiServer string, caData []byte) error {
	g, err := tools.NewGreenhouse(log, apiServer, caData)
	if err != nil {
		log.Error(err, "failed to initialize greenhouse client")
		return err
	}

	mcp.AddTool(server, tools.ClusterTool(), g.GetCluster)
	mcp.AddTool(server, tools.ClusterSchemaTool(), g.GetClusterSchema)

	mcp.AddTool(server, tools.CatalogTool(), g.GetCatalog)
	mcp.AddTool(server, tools.CatalogSchemaTool(), g.GetCatalogSchema)

	mcp.AddTool(server, tools.PluginTool(), g.GetPlugin)
	mcp.AddTool(server, tools.ListPluginsTool(), g.ListPlugins)
	mcp.AddTool(server, tools.PluginSchemaTool(), g.GetPluginSchema)

	mcp.AddTool(server, tools.PluginDefinitionTool(), g.GetPluginDefinition)
	mcp.AddTool(server, tools.PluginDefinitionSchemaTool(), g.GetPluginDefinitionSchema)

	mcp.AddTool(server, tools.DebugPluginTool(), g.DebugPlugin)
	mcp.AddTool(server, tools.DebugClusterTool(), g.DebugCluster)

	server.AddPrompt(tools.DebugClusterPrompt(), tools.DebugClusterPromptHandler)

	return nil
}
