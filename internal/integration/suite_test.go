// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	testEnv      *envtest.Environment
	kubeClient   client.Client
	mcpServerURL string
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(greenhousev1alpha1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(helmv2.AddToScheme(scheme))
	return scheme
}()

func TestMain(testingMain *testing.M) {
	ctrl.SetLogger(zap.New(zap.WriteTo(os.Stderr)))
	logger := ctrl.Log

	tokenFilePath := writeTokenAuthFile(logger)

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "testdata", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	// Configure the API server to accept static bearer tokens so that
	// TokenReview requests from the MCP server can authenticate test requests.
	testEnv.ControlPlane.GetAPIServer().Configure().Append("token-auth-file", tokenFilePath)

	restConfig, err := testEnv.Start()
	if err != nil {
		logger.Error(err, "failed to start envtest environment")
		os.Exit(1)
	}

	kubeClient, err = client.New(restConfig, client.Options{Scheme: testScheme})
	if err != nil {
		logger.Error(err, "failed to create kube client")
		stopEnv(logger)
		os.Exit(1)
	}

	if err := bindClusterAdmin(context.Background(), kubeClient, testUsername); err != nil {
		logger.Error(err, "failed to bind cluster-admin to test user")
		stopEnv(logger)
		os.Exit(1)
	}

	mcpServerURL, err = startMCPServer(logger, restConfig)
	if err != nil {
		logger.Error(err, "failed to start MCP server")
		stopEnv(logger)
		os.Exit(1)
	}

	exitCode := testingMain.Run()

	stopEnv(logger)
	os.Exit(exitCode)
}

func stopEnv(logger interface{ Error(err error, msg string, keysAndValues ...any) }) {
	if err := testEnv.Stop(); err != nil {
		logger.Error(err, "failed to stop envtest environment")
	}
}
