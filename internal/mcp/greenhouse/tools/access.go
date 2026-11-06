// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CheckAccess validates the incoming token via TokenReview + SubjectAccessReview.
// For agent SPIFFE callers it substitutes X-On-Behalf-Of-User/Groups into the SAR
// and returns a per-request client authenticated with the SPIFFE token so that
// downstream CRD calls hit the Greenhouse API server under the agent identity.
// For human callers it returns the shared pod-SA client.
func (g *Greenhouse) CheckAccess(ctx context.Context, req *mcp.CallToolRequest, verb, resource, namespace, name string) (client.Client, error) {
	token := extractToken(req)
	if token == "" {
		return nil, errors.New("no bearer token found in request")
	}

	tokenReview := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: token,
		},
	}
	if err := g.Create(ctx, tokenReview); err != nil {
		g.log.Error(err, "token review failed")
		return nil, err
	}
	if !tokenReview.Status.Authenticated {
		g.log.Info("token not authenticated", "user", tokenReview.Status.User.Username)
		return nil, errors.New("token is not authenticated")
	}

	sarUser := tokenReview.Status.User.Username
	sarGroups := tokenReview.Status.User.Groups
	isAgent := false

	if oboUser, oboGroups := onBehalfOf(req); oboUser != "" {
		sarUser = oboUser
		sarGroups = oboGroups
		isAgent = true
	}

	subjectAccessReview := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			User:   sarUser,
			Groups: sarGroups,
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:      verb,
				Group:     greenhouseGroup,
				Resource:  resource,
				Namespace: namespace,
				Name:      name,
			},
		},
	}
	if err := g.Create(ctx, subjectAccessReview); err != nil {
		g.log.Error(err, "subject access review failed")
		return nil, err
	}
	if !subjectAccessReview.Status.Allowed {
		reason := subjectAccessReview.Status.Reason
		if reason == "" {
			reason = "no reason provided"
		}
		g.log.Info("access denied", "user", sarUser, "groups", sarGroups, "verb", verb, "resource", resource, "namespace", namespace, "name", name, "reason", reason)
		return nil, fmt.Errorf("access denied for user %s: %s", sarUser, reason)
	}

	g.log.Info("access granted", "user", sarUser, "groups", sarGroups, "verb", verb, "resource", resource, "namespace", namespace, "name", name)

	if isAgent {
		agentClient, err := g.factory.ForToken(token)
		if err != nil {
			return nil, fmt.Errorf("building agent client: %w", err)
		}
		return agentClient, nil
	}
	return g.Client, nil
}

// onBehalfOf extracts X-On-Behalf-Of-User and X-On-Behalf-Of-Groups headers.
// Returns empty strings/nil if not present.
func onBehalfOf(req *mcp.CallToolRequest) (user string, groups []string) {
	user = req.Extra.Header.Get(headerOnBehalfOfUser)
	if rawGroups := req.Extra.Header.Get(headerOnBehalfOfGroups); rawGroups != "" {
		groups = strings.Split(rawGroups, ",")
	}
	return
}

// clientForRequest returns a per-request client authenticated with the
// incoming SPIFFE token when on-behalf-of headers are present, or the
// shared pod-SA client for human callers.
func (g *Greenhouse) clientForRequest(req *mcp.CallToolRequest) (client.Client, error) {
	oboUser, _ := onBehalfOf(req)
	if oboUser == "" {
		return g.Client, nil
	}
	token := extractToken(req)
	if token == "" {
		return nil, errors.New("no bearer token found in request")
	}
	return g.factory.ForToken(token)
}

func extractToken(req *mcp.CallToolRequest) string {
	authHeader := req.Extra.Header.Get("Authorization")
	token, ok := strings.CutPrefix(authHeader, "Bearer ")
	if ok {
		return token
	}
	return authHeader
}
