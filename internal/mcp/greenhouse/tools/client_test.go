// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudoperators/greenhouse-mcp/internal/mocks"
)

// newTestGreenhouse creates a Greenhouse backed by the given mock client.
// A dummy ClientFactory is configured so tests exercising agent (OBO) paths
// don't nil-panic when ForToken is called — the factory will fail to build a
// real client against the dummy address, which is the expected behaviour.
func newTestGreenhouse(t *testing.T) (*Greenhouse, *mocks.MockClient) {
	t.Helper()
	mockClient := mocks.NewMockClient(t)
	greenhouse := &Greenhouse{
		Client:  mockClient,
		factory: newClientFactory("https://127.0.0.1:0", nil),
		log:     logr.Discard(),
	}
	return greenhouse, mockClient
}

// reqWithToken builds a *mcp.CallToolRequest whose Extra.Header carries
// the supplied bearer token.
func reqWithToken(token string) *mcp.CallToolRequest {
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	return &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: header},
	}
}

// authenticateUser returns a Create Run callback that marks the TokenReview as authenticated.
func authenticateUser(user string, groups []string) func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		tokenReview, ok := obj.(*authenticationv1.TokenReview)
		if !ok {
			return
		}
		tokenReview.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: user,
				Groups:   groups,
			},
		}
	}
}

// denyToken returns a Create Run callback that marks the TokenReview as not authenticated.
func denyToken() func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		tokenReview, ok := obj.(*authenticationv1.TokenReview)
		if !ok {
			return
		}
		tokenReview.Status = authenticationv1.TokenReviewStatus{Authenticated: false}
	}
}

// allowAccess returns a Create Run callback that allows the SubjectAccessReview.
func allowAccess() func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		subjectAccessReview, ok := obj.(*authorizationv1.SubjectAccessReview)
		if !ok {
			return
		}
		subjectAccessReview.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
	}
}

// denyAccess returns a Create Run callback that denies the SubjectAccessReview.
func denyAccess() func(context.Context, client.Object, ...client.CreateOption) {
	return func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
		subjectAccessReview, ok := obj.(*authorizationv1.SubjectAccessReview)
		if !ok {
			return
		}
		subjectAccessReview.Status = authorizationv1.SubjectAccessReviewStatus{
			Allowed: false,
			Reason:  "forbidden by policy",
		}
	}
}
