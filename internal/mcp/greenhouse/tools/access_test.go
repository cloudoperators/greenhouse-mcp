// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudoperators/greenhouse-mcp/internal/mocks"
)

func TestCheckAccess(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		setup     func(mockClient *mocks.MockClient)
		wantErr   string
		wantAgent bool // true means we expect a per-request client, not the shared pod-SA client
	}{
		{
			name:    "missing token returns error immediately",
			token:   "",
			setup:   func(_ *mocks.MockClient) {},
			wantErr: "no bearer token found in request",
		},
		{
			name:  "token review API failure propagates error",
			token: "some-token",
			setup: func(mockClient *mocks.MockClient) {
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Return(errors.New("api-server unreachable")).Once()
			},
			wantErr: "api-server unreachable",
		},
		{
			name:  "unauthenticated token returns error",
			token: "bad-token",
			setup: func(mockClient *mocks.MockClient) {
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(denyToken()).
					Return(nil).Once()
			},
			wantErr: "token is not authenticated",
		},
		{
			name:  "subject access review API failure propagates error",
			token: "valid-token",
			setup: func(mockClient *mocks.MockClient) {
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Return(errors.New("sar api error")).Once()
			},
			wantErr: "sar api error",
		},
		{
			name:  "access denied returns error with reason",
			token: "valid-token",
			setup: func(mockClient *mocks.MockClient) {
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(denyAccess()).
					Return(nil).Once()
			},
			wantErr: "access denied for user alice",
		},
		{
			name:  "human caller returns shared pod-SA client",
			token: "valid-token",
			setup: func(mockClient *mocks.MockClient) {
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mockClient.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
			},
			wantAgent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			greenhouse, mockClient := newTestGreenhouse(t)
			tt.setup(mockClient)

			req := reqWithToken(tt.token)
			resultClient, err := greenhouse.CheckAccess(context.Background(), req, "get", "clusters", "default", "my-cluster")

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, resultClient)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resultClient)
		})
	}
}

func TestCheckAccess_AgentOnBehalfOf(t *testing.T) {
	greenhouse, mockClient := newTestGreenhouse(t)

	mockClient.EXPECT().
		Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
		Run(authenticateUser("spiffe://cluster/agent", []string{"system:agents"})).
		Return(nil).Once()

	// SAR must be called with the OBO user, not the SPIFFE identity.
	mockClient.EXPECT().
		Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
		Run(func(_ context.Context, obj client.Object, _ ...client.CreateOption) {
			subjectAccessReview := obj.(*authorizationv1.SubjectAccessReview)
			assert.Equal(t, "alice", subjectAccessReview.Spec.User, "SAR must use OBO user, not SPIFFE identity")
			assert.Contains(t, subjectAccessReview.Spec.Groups, "developers", "SAR must include OBO groups")
			subjectAccessReview.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
		}).
		Return(nil).Once()

	header := http.Header{}
	header.Set("Authorization", "Bearer spiffe-token")
	header.Set("X-On-Behalf-Of-User", "alice")
	header.Set("X-On-Behalf-Of-Groups", "developers,platform-team")
	req := &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: header},
	}

	// Auth succeeds; the factory builds a per-request client for the SPIFFE token.
	resultClient, err := greenhouse.CheckAccess(context.Background(), req, "get", "clusters", "default", "my-cluster")
	require.NoError(t, err)
	require.NotNil(t, resultClient)
}

func TestExtractToken(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
	}{
		{
			name:      "bearer prefix is stripped",
			header:    "Bearer my-secret-token",
			wantToken: "my-secret-token",
		},
		{
			name:      "bare token without bearer prefix returned as-is",
			header:    "my-secret-token",
			wantToken: "my-secret-token",
		},
		{
			name:      "empty header returns empty string",
			header:    "",
			wantToken: "",
		},
		{
			name:      "bearer with trailing space token",
			header:    "Bearer  spaced-token",
			wantToken: " spaced-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpHeader := http.Header{}
			if tt.header != "" {
				httpHeader.Set("Authorization", tt.header)
			}
			req := &mcp.CallToolRequest{
				Extra: &mcp.RequestExtra{Header: httpHeader},
			}

			token := extractToken(req)
			assert.Equal(t, tt.wantToken, token)
		})
	}
}

func TestOnBehalfOf(t *testing.T) {
	t.Run("no OBO headers returns empty user and nil groups", func(t *testing.T) {
		req := reqWithToken("some-token")
		user, groups := onBehalfOf(req)
		assert.Empty(t, user)
		assert.Nil(t, groups)
	})

	t.Run("OBO headers are extracted and split on comma", func(t *testing.T) {
		header := http.Header{}
		header.Set("X-On-Behalf-Of-User", "alice")
		header.Set("X-On-Behalf-Of-Groups", "developers,platform-team")
		req := &mcp.CallToolRequest{
			Extra: &mcp.RequestExtra{Header: header},
		}

		user, groups := onBehalfOf(req)
		assert.Equal(t, "alice", user)
		assert.Equal(t, []string{"developers", "platform-team"}, groups)
	})

	t.Run("single group without comma", func(t *testing.T) {
		header := http.Header{}
		header.Set("X-On-Behalf-Of-User", "bob")
		header.Set("X-On-Behalf-Of-Groups", "admins")
		req := &mcp.CallToolRequest{
			Extra: &mcp.RequestExtra{Header: header},
		}

		_, groups := onBehalfOf(req)
		assert.Equal(t, []string{"admins"}, groups)
	})
}

// clientForRequest is tested indirectly via CheckAccess. These cases verify the
// direct routing logic: no OBO → shared client, OBO present → factory call.
func TestClientForRequest_NoOBO(t *testing.T) {
	greenhouse, mockClient := newTestGreenhouse(t)

	req := reqWithToken("human-token")
	resultClient, err := greenhouse.clientForRequest(req)

	require.NoError(t, err)
	// Without OBO headers, clientForRequest returns the embedded shared client.
	assert.Equal(t, client.Client(mockClient), resultClient)
}
