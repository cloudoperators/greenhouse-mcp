// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package greenhouse

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cloudoperators/greenhouse-mcp/internal/mcp/greenhouse/tools"
)

func RegisterTools(log logr.Logger, server *mcp.Server, apiServer string) error {
	greenhouse, err := tools.NewGreenhouse(log, apiServer)
	if err != nil {
		return fmt.Errorf("initializing greenhouse client: %w", err)
	}

	registerToolHandlers(server, greenhouse)
	return nil
}

func registerToolHandlers(server *mcp.Server, greenhouse *tools.Greenhouse) {
	mcp.AddTool(server, tools.ClusterTool(), greenhouse.GetCluster)
}
