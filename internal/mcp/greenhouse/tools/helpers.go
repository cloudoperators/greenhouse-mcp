// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"encoding/json"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetArgs holds the name/namespace pair common to all single-resource tool calls.
type GetArgs struct {
	Name      string `json:"name" jsonschema:"name of the resource to retrieve"`
	Namespace string `json:"namespace" jsonschema:"namespace the resource lives in (the Greenhouse organization)"`
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
