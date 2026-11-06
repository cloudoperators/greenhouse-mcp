// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetArgs holds the name/namespace pair common to all single-resource tool calls.
type GetArgs struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// getCRDSchema fetches the named CRD and returns its versions array as JSON.
func (g *Greenhouse) getCRDSchema(ctx context.Context, c client.Client, crdName string) (*mcp.CallToolResult, any, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	key := client.ObjectKey{Name: crdName}
	if err := c.Get(ctx, key, crd); err != nil {
		g.log.Error(err, "failed to get CRD", "name", crdName)
		return nil, nil, kubeErr(err)
	}

	jsonBytes, err := json.MarshalIndent(crd.Spec.Versions, "", "  ")
	if err != nil {
		g.log.Error(err, "failed to marshal CRD schema to JSON")
		return nil, nil, kubeErr(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonBytes)},
		},
	}, nil, nil
}

// cleanAnnotations removes the kubectl.kubernetes.io/last-applied-configuration
// annotation to avoid sending large, noisy blobs in tool responses.
func cleanAnnotations(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		obj.SetAnnotations(annotations)
	}
}

// kubeErr sanitizes a Kubernetes client error: maps 403 Forbidden to a plain
// "permission denied" so internal API details are not exposed to callers.
func kubeErr(err error) error {
	if apierrors.IsForbidden(err) {
		return errors.New("permission denied")
	}
	return err
}

// MarshalToJSON serialises obj to indented JSON, returning the error message
// as a string if marshalling fails (tool handlers always return text content).
func MarshalToJSON(obj any) string {
	jsonBytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(jsonBytes)
}
