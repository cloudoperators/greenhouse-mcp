// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"net/http"
	"testing"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudoperators/greenhouse-mcp/internal/mocks"
)

// newTestGreenhouse creates a Greenhouse instance backed by the given mock client.
func newTestGreenhouse(t *testing.T) (*Greenhouse, *mocks.MockClient) {
	t.Helper()
	mc := mocks.NewMockClient(t)
	g := &Greenhouse{
		Client: mc,
		log:    logr.Discard(),
	}
	return g, mc
}

// reqWithToken builds a *mcp.CallToolRequest whose Extra.Header carries
// the supplied bearer token.
func reqWithToken(token string) *mcp.CallToolRequest {
	h := http.Header{}
	if token != "" {
		h.Set("Authorization", "Bearer "+token)
	}
	return &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: h},
	}
}

// --- mock helpers to simulate K8s API behaviour on Create calls ---

// authenticateUser returns a Run callback that marks the TokenReview as authenticated.
func authenticateUser(user string, groups []string) func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		tr, ok := obj.(*authenticationv1.TokenReview)
		if !ok {
			return
		}
		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: user,
				Groups:   groups,
			},
		}
	}
}

// denyToken returns a Run callback that marks the TokenReview as not authenticated.
func denyToken() func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		tr, ok := obj.(*authenticationv1.TokenReview)
		if !ok {
			return
		}
		tr.Status = authenticationv1.TokenReviewStatus{Authenticated: false}
	}
}

// allowAccess returns a Run callback that allows the SubjectAccessReview.
func allowAccess() func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		sar, ok := obj.(*authorizationv1.SubjectAccessReview)
		if !ok {
			return
		}
		sar.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
	}
}

// denyAccess returns a Run callback that denies the SubjectAccessReview.
func denyAccess() func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		sar, ok := obj.(*authorizationv1.SubjectAccessReview)
		if !ok {
			return
		}
		sar.Status = authorizationv1.SubjectAccessReviewStatus{
			Allowed: false,
			Reason:  "forbidden by policy",
		}
	}
}

// populateCatalog returns a Run callback that fills in the Catalog object
// returned by client.Get.
func populateCatalog(name, namespace string) func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) {
	return func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
		cat, ok := obj.(*greenhousev1alpha1.Catalog)
		if !ok {
			return
		}
		cat.ObjectMeta = metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		}
		cat.Spec = greenhousev1alpha1.CatalogSpec{
			Sources: []greenhousev1alpha1.CatalogSource{
				{
					Repository: "https://github.com/example/plugins",
					Resources:  []string{"plugins/*.yaml"},
				},
			},
		}
	}
}

func TestGetCatalog(t *testing.T) {
	tests := []struct {
		name                 string
		token                string
		args                 GetArgs
		setup                func(mc *mocks.MockClient)
		wantErr              string
		wantJSON             string // substring expected in the result text
		checkAnnotationClean bool   // verify kubectl annotation was removed
	}{
		{
			name:    "missing token returns error",
			token:   "",
			args:    GetArgs{Name: "my-catalog", Namespace: "my-org"},
			setup:   func(_ *mocks.MockClient) {},
			wantErr: "no bearer token found in request",
		},
		{
			name:  "unauthenticated token returns error",
			token: "bad-token",
			args:  GetArgs{Name: "my-catalog", Namespace: "my-org"},
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
			args:  GetArgs{Name: "my-catalog", Namespace: "my-org"},
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
			args:  GetArgs{Name: "my-catalog", Namespace: "my-org"},
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
			args:  GetArgs{Name: "missing-catalog", Namespace: "my-org"},
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
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Catalog"), mock.Anything).
					Return(errors.New("not found")).Once()
			},
			wantErr: "not found",
		},
		{
			name:  "successful catalog retrieval",
			token: "valid-token",
			args:  GetArgs{Name: "my-catalog", Namespace: "my-org"},
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
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Catalog"), mock.Anything).
					Run(populateCatalog("my-catalog", "my-org")).
					Return(nil).Once()
			},
			wantJSON: `"name": "my-catalog"`,
		},
		{
			name:  "annotations are cleaned",
			token: "valid-token",
			args:  GetArgs{Name: "annotated-catalog", Namespace: "my-org"},
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
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Catalog"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						cat := obj.(*greenhousev1alpha1.Catalog)
						cat.ObjectMeta = metav1.ObjectMeta{
							Name:      "annotated-catalog",
							Namespace: "my-org",
							Annotations: map[string]string{
								"kubectl.kubernetes.io/last-applied-configuration": `{"big":"blob"}`,
								"custom-annotation": "keep-me",
							},
						}
					}).
					Return(nil).Once()
			},
			wantJSON:             `"custom-annotation": "keep-me"`,
			checkAnnotationClean: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, mc := newTestGreenhouse(t)
			tt.setup(mc)

			req := reqWithToken(tt.token)
			result, _, err := g.GetCatalog(context.Background(), req, tt.args)

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
		})
	}
}
