// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"fmt"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func DebugClusterPrompt() *mcpsdk.Prompt {
	return &mcpsdk.Prompt{
		Name:        "debug_cluster",
		Description: "Debug a Greenhouse Cluster — calls debug_cluster and explains the findings",
		Arguments: []*mcpsdk.PromptArgument{
			{Name: "name", Description: "Cluster name", Required: true},
			{Name: "organization", Description: "Greenhouse organization (namespace)", Required: true},
		},
	}
}

func DebugClusterPromptHandler(_ context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
	name := req.Params.Arguments["name"]
	org := req.Params.Arguments["organization"]
	if name == "" || org == "" {
		return nil, errors.New("name and organization are required")
	}
	return &mcpsdk.GetPromptResult{
		Messages: []*mcpsdk.PromptMessage{
			{
				Role: "user",
				Content: &mcpsdk.TextContent{
					Text: fmt.Sprintf("Debug cluster %q in organization %q. Call debug_cluster and explain what is wrong and what should be done next.", name, org),
				},
			},
		},
	}, nil
}
