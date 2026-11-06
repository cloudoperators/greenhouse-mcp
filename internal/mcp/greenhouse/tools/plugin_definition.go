// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (g *Greenhouse) GetPluginDefinitionSchema(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
	c, err := g.clientForRequest(req)
	if err != nil {
		return nil, nil, kubeErr(err)
	}
	return g.getCRDSchema(ctx, c, pluginDefinitionSchema)
}

func (g *Greenhouse) GetPluginDefinition(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
	c, err := g.CheckAccess(ctx, req, "get", "plugindefinitions", args.Namespace, args.Name)
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	pd := &greenhousev1alpha1.PluginDefinition{}
	key := client.ObjectKey{Name: args.Name, Namespace: args.Namespace}
	if err := c.Get(ctx, key, pd); err != nil {
		g.log.Error(err, "failed to get PluginDefinition", "name", args.Name, "namespace", args.Namespace)
		return nil, nil, kubeErr(err)
	}

	pd.SetManagedFields(nil)
	cleanAnnotations(pd)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: MarshalToJSON(pd)},
		},
	}, nil, nil
}
