// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
)

func TestKubeErr(t *testing.T) {
	t.Run("forbidden error is sanitized to permission denied", func(t *testing.T) {
		forbiddenErr := apierrors.NewForbidden(schema.GroupResource{Group: "greenhouse.sap", Resource: "clusters"}, "my-cluster", errors.New("access denied"))
		result := kubeErr(forbiddenErr)
		assert.EqualError(t, result, "permission denied")
	})

	t.Run("non-forbidden error is returned unchanged", func(t *testing.T) {
		notFoundErr := apierrors.NewNotFound(schema.GroupResource{Resource: "clusters"}, "my-cluster")
		result := kubeErr(notFoundErr)
		assert.Equal(t, notFoundErr, result)
	})

	t.Run("generic error is returned unchanged", func(t *testing.T) {
		genericErr := errors.New("something went wrong")
		result := kubeErr(genericErr)
		assert.Equal(t, genericErr, result)
	})
}

func TestMarshalToJSON(t *testing.T) {
	t.Run("struct is marshalled to indented JSON", func(t *testing.T) {
		type sample struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		result := MarshalToJSON(sample{Name: "alice", Age: 30})
		assert.Contains(t, result, `"name": "alice"`)
		assert.Contains(t, result, `"age": 30`)
	})

	t.Run("nil serializes to null", func(t *testing.T) {
		result := MarshalToJSON(nil)
		assert.Equal(t, "null", result)
	})

	t.Run("slice serializes as JSON array", func(t *testing.T) {
		result := MarshalToJSON([]string{"a", "b"})
		assert.Contains(t, result, `"a"`)
		assert.Contains(t, result, `"b"`)
	})
}

func TestCleanAnnotations(t *testing.T) {
	t.Run("kubectl annotation is removed, other annotations are kept", func(t *testing.T) {
		cluster := &greenhousev1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-cluster",
				Annotations: map[string]string{
					"kubectl.kubernetes.io/last-applied-configuration": `{"big":"blob"}`,
					"custom/annotation": "keep-me",
				},
			},
		}

		cleanAnnotations(cluster)

		annotations := cluster.GetAnnotations()
		assert.NotContains(t, annotations, "kubectl.kubernetes.io/last-applied-configuration")
		assert.Equal(t, "keep-me", annotations["custom/annotation"])
	})

	t.Run("object with no annotations is not modified", func(t *testing.T) {
		cluster := &greenhousev1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "bare-cluster"},
		}
		cleanAnnotations(cluster)
		assert.Nil(t, cluster.GetAnnotations())
	})

	t.Run("object with annotations but no kubectl annotation is not modified", func(t *testing.T) {
		cluster := &greenhousev1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "annotated-cluster",
				Annotations: map[string]string{"team": "platform"},
			},
		}
		cleanAnnotations(cluster)
		assert.Equal(t, map[string]string{"team": "platform"}, cluster.GetAnnotations())
	})
}
