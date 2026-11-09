// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"net/http"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(greenhousev1alpha1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(helmv2.AddToScheme(scheme))
}

const (
	greenhouseGroup        = "greenhouse.sap"
	catalogSchema          = "catalogs.greenhouse.sap"
	clusterSchema          = "clusters.greenhouse.sap"
	pluginSchema           = "plugins.greenhouse.sap"
	pluginDefinitionSchema = "plugindefinitions.greenhouse.sap"

	headerOnBehalfOfUser   = "X-On-Behalf-Of-User"
	headerOnBehalfOfGroups = "X-On-Behalf-Of-Groups"
)

// ClientFactory builds per-request clients using a static host + CA. The
// scheme and transport wrapper are constructed once; only the bearer token
// varies per call.
type ClientFactory struct {
	apiServer string
	caData    []byte
}

func newClientFactory(apiServer string, caData []byte) *ClientFactory {
	return &ClientFactory{apiServer: apiServer, caData: caData}
}

// ForToken returns a client authenticated with the given bearer token.
func (f *ClientFactory) ForToken(token string) (client.Client, error) {
	cfg := &rest.Config{ //nolint:modernize // embedded field type cannot be elided for rest.Config.TLSClientConfig
		Host:        f.apiServer,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: f.caData,
		},
		WrapTransport: func(roundTripper http.RoundTripper) http.RoundTripper {
			return otelhttp.NewTransport(roundTripper)
		},
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

// Greenhouse is the domain object that wires together the shared pod-SA client,
// the per-request ClientFactory, and the logger used across all tool handlers.
type Greenhouse struct {
	client.Client
	factory *ClientFactory
	log     logr.Logger
}

// NewGreenhouse constructs a Greenhouse by resolving the REST config through
// the standard in-cluster → kubeconfig discovery chain.
func NewGreenhouse(log logr.Logger, apiServer string, caData []byte) (*Greenhouse, error) {
	cfg, err := getRESTConfig(log, apiServer, caData)
	if err != nil {
		log.Error(err, "failed to build REST config")
		return nil, err
	}

	kubeClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "failed to create k8s client")
		return nil, err
	}

	return &Greenhouse{
		Client:  kubeClient,
		factory: newClientFactory(apiServer, caData),
		log:     log,
	}, nil
}

// NewGreenhouseWithConfig constructs a Greenhouse from an already-resolved
// *rest.Config, bypassing in-cluster / kubeconfig discovery.
// Intended for integration tests that inject an envtest config directly.
func NewGreenhouseWithConfig(log logr.Logger, cfg *rest.Config) (*Greenhouse, error) {
	cfg.WrapTransport = func(roundTripper http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(roundTripper)
	}

	kubeClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "failed to create k8s client from config")
		return nil, err
	}

	return &Greenhouse{
		Client:  kubeClient,
		factory: newClientFactory(cfg.Host, cfg.CAData),
		log:     log,
	}, nil
}

func getRESTConfig(log logr.Logger, apiServer string, caData []byte) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		log.V(2).Info("in-cluster config detected, overriding host with greenhouse settings")
		cfg.Host = apiServer
		cfg.TLSClientConfig = rest.TLSClientConfig{CAData: caData}
		cfg.WrapTransport = func(roundTripper http.RoundTripper) http.RoundTripper {
			return otelhttp.NewTransport(roundTripper)
		}
		return cfg, nil
	}

	log.V(2).Info("in-cluster config not available, falling back to kubeconfig", "reason", err)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	cfg, err = kubeConfig.ClientConfig()
	if err != nil {
		log.Error(err, "failed to load kubeconfig")
		return nil, err
	}
	// In the kubeconfig path the host and credentials are taken as-is from the
	// active context. This path is only used for local dev and integration tests —
	// in production the in-cluster branch above runs instead.
	// Note: --greenhouse-api-server is intentionally NOT applied here because the
	// kubeconfig context already points at the correct cluster (including its own
	// CA and client credentials). Overriding only the host would break TLS.
	if len(caData) > 0 {
		cfg.CAData = caData
	}
	cfg.WrapTransport = func(roundTripper http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(roundTripper)
	}
	return cfg, nil
}
