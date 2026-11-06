// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// testBearerToken is the static token written to the token auth file.
	// The envtest API server accepts it and authenticates it as testUsername.
	testBearerToken = "integration-test-token"
	testUsername    = "integration-test-user"
	testUserUID     = "integration-test-uid"
)

// writeTokenAuthFile writes a CSV token auth file that the envtest API server
// reads at startup to recognise static bearer tokens.
// Format per line: token,username,uid,groups
func writeTokenAuthFile(logger logr.Logger) string {
	tokenFileDir, err := os.MkdirTemp("", "envtest-tokens-*")
	if err != nil {
		logger.Error(err, "failed to create temp dir for token auth file")
		os.Exit(1)
	}

	tokenFilePath := filepath.Join(tokenFileDir, "tokens.csv")
	tokenFileContent := fmt.Sprintf("%s,%s,%s,\n", testBearerToken, testUsername, testUserUID)

	if err := os.WriteFile(tokenFilePath, []byte(tokenFileContent), 0600); err != nil {
		logger.Error(err, "failed to write token auth file", "path", tokenFilePath)
		os.Exit(1)
	}

	return tokenFilePath
}

// bindClusterAdmin creates a ClusterRoleBinding that grants cluster-admin to
// the test user so that SubjectAccessReview approves all resource requests.
func bindClusterAdmin(ctx context.Context, kubeClient client.Client, username string) error {
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "integration-test-cluster-admin",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "cluster-admin",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     rbacv1.UserKind,
				APIGroup: rbacv1.GroupName,
				Name:     username,
			},
		},
	}

	return kubeClient.Create(ctx, clusterRoleBinding)
}
