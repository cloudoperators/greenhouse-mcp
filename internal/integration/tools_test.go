// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// callTool sends a single MCP tools/call request to the integration server
// and returns the first text content from the response.
func callTool(t *testing.T, toolName string, arguments map[string]any) string {
	t.Helper()

	requestBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	requestBytes, err := json.Marshal(requestBody)
	require.NoError(t, err, "marshalling MCP request")

	httpRequest, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		mcpServerURL,
		bytes.NewReader(requestBytes),
	)
	require.NoError(t, err, "creating HTTP request")

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json, text/event-stream")
	httpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testBearerToken))

	httpResponse, err := http.DefaultClient.Do(httpRequest)
	require.NoError(t, err, "executing HTTP request")
	defer httpResponse.Body.Close() //nolint:errcheck

	responseBytes, err := io.ReadAll(httpResponse.Body)
	require.NoError(t, err, "reading response body")
	require.Equal(t, http.StatusOK, httpResponse.StatusCode,
		"unexpected HTTP status: %s", string(responseBytes))

	// The streamable HTTP transport responds with SSE when the Accept header
	// includes text/event-stream. Extract the JSON payload from the data line.
	jsonPayload := extractSSEData(t, responseBytes)

	var mcpResponse struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	require.NoError(t, json.Unmarshal(jsonPayload, &mcpResponse), "unmarshalling MCP response")

	if mcpResponse.Error != nil {
		t.Fatalf("MCP error response: %s", mcpResponse.Error.Message)
	}

	require.NotEmpty(t, mcpResponse.Result.Content, "MCP response has no content")
	return mcpResponse.Result.Content[0].Text
}

// extractSSEData parses an SSE response body and returns the JSON payload
// from the first "data:" line.
func extractSSEData(t *testing.T, body []byte) []byte {
	t.Helper()

	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			return bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		}
	}

	t.Fatalf("no data line found in SSE response: %s", string(body))
	return nil
}

func TestGetCluster_Integration(t *testing.T) {
	ctx := context.Background()

	cluster := &greenhousev1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: greenhousev1alpha1.ClusterSpec{
			AccessMode: greenhousev1alpha1.ClusterAccessModeDirect,
		},
	}
	require.NoError(t, kubeClient.Create(ctx, cluster))
	t.Cleanup(func() { _ = kubeClient.Delete(ctx, cluster) })

	responseText := callTool(t, "get_cluster", map[string]any{
		"name":      "test-cluster",
		"namespace": "default",
	})

	assert.Contains(t, responseText, "test-cluster")
	assert.Contains(t, responseText, "direct")
}

func TestGetPlugin_Integration(t *testing.T) {
	ctx := context.Background()

	plugin := &greenhousev1alpha1.Plugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-plugin",
			Namespace: "default",
		},
		Spec: greenhousev1alpha1.PluginSpec{
			PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{
				Name: "alerts",
			},
			ClusterName: "test-cluster",
		},
	}
	require.NoError(t, kubeClient.Create(ctx, plugin))
	t.Cleanup(func() { _ = kubeClient.Delete(ctx, plugin) })

	responseText := callTool(t, "get_plugin", map[string]any{
		"name":      "test-plugin",
		"namespace": "default",
	})

	assert.Contains(t, responseText, "test-plugin")
	assert.Contains(t, responseText, "alerts")
}

func TestGetCatalog_Integration(t *testing.T) {
	ctx := context.Background()

	catalog := &greenhousev1alpha1.Catalog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-catalog",
			Namespace: "default",
		},
		Spec: greenhousev1alpha1.CatalogSpec{
			Sources: []greenhousev1alpha1.CatalogSource{
				{
					Repository: "https://github.com/example/plugins",
					Resources:  []string{"plugins/*.yaml"},
				},
			},
		},
	}
	require.NoError(t, kubeClient.Create(ctx, catalog))
	t.Cleanup(func() { _ = kubeClient.Delete(ctx, catalog) })

	responseText := callTool(t, "get_catalog", map[string]any{
		"name":      "test-catalog",
		"namespace": "default",
	})

	assert.Contains(t, responseText, "test-catalog")
}

func TestGetClusterSchema_Integration(t *testing.T) {
	responseText := callTool(t, "get_cluster_schema", map[string]any{})

	// The schema tool returns the CRD versions array — must be valid JSON.
	var versions []any
	require.NoError(t, json.Unmarshal([]byte(responseText), &versions),
		"cluster schema response is not valid JSON array")
	assert.NotEmpty(t, versions, "cluster schema has no versions")
}

func TestListPlugins_Integration(t *testing.T) {
	ctx := context.Background()

	plugin := &greenhousev1alpha1.Plugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "listed-plugin",
			Namespace: "default",
		},
		Spec: greenhousev1alpha1.PluginSpec{
			PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{
				Name: "kube-monitoring",
			},
			ClusterName: "test-cluster",
		},
	}
	require.NoError(t, kubeClient.Create(ctx, plugin))
	t.Cleanup(func() { _ = kubeClient.Delete(ctx, plugin) })

	responseText := callTool(t, "list_plugins", map[string]any{
		"namespace": "default",
	})

	assert.Contains(t, responseText, "listed-plugin")
}
