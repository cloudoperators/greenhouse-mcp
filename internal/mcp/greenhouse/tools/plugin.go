package tools

import (
	"context"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ListArgs struct {
	Namespace string `json:"namespace"`
}

func (g *Greenhouse) GetPlugin(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
	c, err := g.CheckAccess(ctx, req, "get", "plugins", args.Namespace, args.Name)
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	plugin := &greenhousev1alpha1.Plugin{}
	key := client.ObjectKey{Name: args.Name, Namespace: args.Namespace}
	if err := c.Get(ctx, key, plugin); err != nil {
		g.log.Error(err, "failed to get plugin", "name", args.Name, "namespace", args.Namespace)
		return nil, nil, kubeErr(err)
	}

	plugin.SetManagedFields(nil)
	cleanAnnotations(plugin)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: MarshalToJSON(plugin)},
		},
	}, nil, nil
}

func (g *Greenhouse) ListPlugins(ctx context.Context, req *mcp.CallToolRequest, args ListArgs) (*mcp.CallToolResult, any, error) {
	c, err := g.CheckAccess(ctx, req, "list", "plugins", args.Namespace, "")
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	list := &greenhousev1alpha1.PluginList{}
	if err := c.List(ctx, list, client.InNamespace(args.Namespace)); err != nil {
		g.log.Error(err, "failed to list plugins", "namespace", args.Namespace)
		return nil, nil, kubeErr(err)
	}

	for i := range list.Items {
		list.Items[i].SetManagedFields(nil)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: MarshalToJSON(list.Items)},
		},
	}, nil, nil
}

func (g *Greenhouse) GetPluginSchema(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	c, err := g.clientForRequest(req)
	if err != nil {
		return nil, nil, kubeErr(err)
	}
	return g.getCRDSchema(ctx, c, pluginSchema)
}
