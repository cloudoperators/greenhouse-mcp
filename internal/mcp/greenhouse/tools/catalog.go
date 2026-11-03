package tools

import (
	"context"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (g *Greenhouse) GetCatalog(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
	c, err := g.CheckAccess(ctx, req, "get", "catalogs", args.Namespace, args.Name)
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	catalog := &greenhousev1alpha1.Catalog{}
	key := client.ObjectKey{Name: args.Name, Namespace: args.Namespace}
	if err := c.Get(ctx, key, catalog); err != nil {
		g.log.Error(err, "failed to get catalog", "name", args.Name, "namespace", args.Namespace)
		return nil, nil, kubeErr(err)
	}

	catalog.SetManagedFields(nil)
	cleanAnnotations(catalog)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: MarshalToJSON(catalog)},
		},
	}, nil, nil
}

func (g *Greenhouse) GetCatalogSchema(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	c, err := g.clientForRequest(req)
	if err != nil {
		return nil, nil, err
	}
	return g.getCRDSchema(ctx, c, catalogSchema)
}
