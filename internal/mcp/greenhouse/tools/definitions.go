// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

func ClusterTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_cluster",
		Title: "Get Greenhouse Cluster",
		Description: `Retrieves a Greenhouse Cluster resource by name and namespace. A Cluster represents a remote Kubernetes cluster onboarded into the Greenhouse platform.
Returns JSON with: metadata, spec (accessMode), status (kubernetesVersion, bearerTokenValidity, statusConditions with Ready/HeadscaleReady states, nodes list).
Use this to check cluster health, connectivity, Kubernetes version, or node inventory. Requires 'name' (cluster name) and 'namespace' (Greenhouse organization).`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}
