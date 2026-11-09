// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"testing"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudoperators/greenhouse-mcp/internal/mocks"
)

// populatePluginDefinition returns a Run callback that fills in the PluginDefinition
// object returned by client.Get.
func populatePluginDefinition(name, namespace string) func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) {
	return func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
		pd, ok := obj.(*greenhousev1alpha1.PluginDefinition)
		if !ok {
			return
		}
		pd.ObjectMeta = metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		}
		pd.Spec = greenhousev1alpha1.PluginDefinitionSpec{
			DisplayName: "Test Plugin Definition",
			Description: "A test plugin definition for unit testing",
			Version:     "1.0.0",
			Icon:        "autoScaling",
			HelmChart: &greenhousev1alpha1.HelmChartReference{
				Name:       "test-chart",
				Repository: "https://charts.example.com",
				Version:    "0.1.0",
			},
		}
	}
}

func TestGetPluginDefinition(t *testing.T) {
	tests := []struct {
		name                  string
		token                 string
		args                  GetArgs
		setup                 func(mc *mocks.MockClient)
		wantErr               string
		wantJSON              string
		checkAnnotationClean  bool
		checkManagedFieldGone bool
	}{
		{
			name:    "missing token returns error",
			token:   "",
			args:    GetArgs{Name: "my-plugindef", Namespace: "my-org"},
			setup:   func(_ *mocks.MockClient) {},
			wantErr: "no bearer token found in request",
		},
		{
			name:  "unauthenticated token returns error",
			token: "bad-token",
			args:  GetArgs{Name: "my-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(denyToken()).
					Return(nil).Once()
			},
			wantErr: "token is not authenticated",
		},
		{
			name:  "token review API failure returns error",
			token: "some-token",
			args:  GetArgs{Name: "my-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Return(errors.New("api-server unreachable")).Once()
			},
			wantErr: "api-server unreachable",
		},
		{
			name:  "access denied returns error",
			token: "valid-token",
			args:  GetArgs{Name: "my-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(denyAccess()).
					Return(nil).Once()
			},
			wantErr: "access denied for user alice",
		},
		{
			name:  "client.Get failure returns error",
			token: "valid-token",
			args:  GetArgs{Name: "missing-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Return(errors.New("not found")).Once()
			},
			wantErr: "not found",
		},
		{
			name:  "successful plugin definition retrieval",
			token: "valid-token",
			args:  GetArgs{Name: "my-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(populatePluginDefinition("my-plugindef", "my-org")).
					Return(nil).Once()
			},
			wantJSON: `"name": "my-plugindef"`,
		},
		{
			name:  "helm chart fields are present in output",
			token: "valid-token",
			args:  GetArgs{Name: "helm-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(populatePluginDefinition("helm-plugindef", "my-org")).
					Return(nil).Once()
			},
			wantJSON: `"repository": "https://charts.example.com"`,
		},
		{
			name:  "annotations are cleaned",
			token: "valid-token",
			args:  GetArgs{Name: "annotated-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("bob", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{
							Name:      "annotated-plugindef",
							Namespace: "my-org",
							Annotations: map[string]string{
								"kubectl.kubernetes.io/last-applied-configuration": `{"big":"blob"}`,
								"custom-annotation": "keep-me",
							},
						}
						pd.Spec = greenhousev1alpha1.PluginDefinitionSpec{
							Version: "2.0.0",
						}
					}).
					Return(nil).Once()
			},
			wantJSON:             `"custom-annotation": "keep-me"`,
			checkAnnotationClean: true,
		},
		{
			name:  "managed fields are removed",
			token: "valid-token",
			args:  GetArgs{Name: "managed-plugindef", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{
							Name:      "managed-plugindef",
							Namespace: "my-org",
							ManagedFields: []metav1.ManagedFieldsEntry{
								{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
							},
						}
						pd.Spec = greenhousev1alpha1.PluginDefinitionSpec{
							Version: "3.0.0",
						}
					}).
					Return(nil).Once()
			},
			wantJSON:              `"name": "managed-plugindef"`,
			checkManagedFieldGone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, mc := newTestGreenhouse(t)
			tt.setup(mc)

			req := reqWithToken(tt.token)
			result, _, err := g.GetPluginDefinition(context.Background(), req, tt.args)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.Content, 1)
			text := result.Content[0].(*mcp.TextContent).Text
			assert.Contains(t, text, tt.wantJSON)

			if tt.checkAnnotationClean {
				assert.NotContains(t, text, "last-applied-configuration")
			}
			if tt.checkManagedFieldGone {
				assert.NotContains(t, text, "managedFields")
			}
		})
	}
}
