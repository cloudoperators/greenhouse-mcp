package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/goccy/go-json"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

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
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return otelhttp.NewTransport(rt)
		},
	}
	return client.New(cfg, client.Options{Scheme: scheme})
}

type Greenhouse struct {
	client.Client
	factory *ClientFactory
	log     logr.Logger
}

func NewGreenhouse(log logr.Logger, apiServer string, caData []byte) (*Greenhouse, error) {
	cfg, err := getRESTConfig(log, apiServer, caData)
	if err != nil {
		log.Error(err, "failed to build REST config")
		return nil, err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "failed to create k8s client")
		return nil, err
	}

	return &Greenhouse{
		Client:  c,
		factory: newClientFactory(apiServer, caData),
		log:     log,
	}, nil
}

func getRESTConfig(log logr.Logger, apiServer string, caData []byte) (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		log.V(2).Info("in-cluster config detected, overriding host with greenhouse settings")
		cfg.Host = apiServer
		cfg.TLSClientConfig = rest.TLSClientConfig{CAData: caData}
		cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			return otelhttp.NewTransport(rt)
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
	if len(caData) > 0 {
		cfg.CAData = caData
	}
	cfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return otelhttp.NewTransport(rt)
	}
	return cfg, nil
}

// CheckAccess validates the incoming token via TokenReview + SubjectAccessReview.
// For agent SPIFFE callers it substitutes X-On-Behalf-Of-User/Groups into the SAR
// and returns a per-request client authenticated with the SPIFFE token so that
// downstream CRD calls hit play-ground under the agent identity.
// For human callers it returns the shared pod-SA client.
func (g *Greenhouse) CheckAccess(ctx context.Context, req *mcp.CallToolRequest, verb, resource, namespace, name string) (client.Client, error) {
	token := extractToken(req)
	if token == "" {
		return nil, errors.New("no bearer token found in request")
	}

	tr := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: token,
		},
	}
	if err := g.Create(ctx, tr); err != nil {
		g.log.Error(err, "token review failed")
		return nil, err
	}
	if !tr.Status.Authenticated {
		g.log.Info("token not authenticated", "user", tr.Status.User.Username)
		return nil, errors.New("token is not authenticated")
	}

	sarUser := tr.Status.User.Username
	sarGroups := tr.Status.User.Groups
	isAgent := false

	if oboUser, oboGroups := onBehalfOf(req); oboUser != "" {
		sarUser = oboUser
		sarGroups = oboGroups
		isAgent = true
	}

	sar := &authorizationv1.SubjectAccessReview{
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
	if err := g.Create(ctx, sar); err != nil {
		g.log.Error(err, "subject access review failed")
		return nil, err
	}
	if !sar.Status.Allowed {
		reason := sar.Status.Reason
		if reason == "" {
			reason = "no reason provided"
		}
		g.log.Info("access denied", "user", sarUser, "groups", sarGroups, "verb", verb, "resource", resource, "namespace", namespace, "name", name, "reason", reason)
		return nil, fmt.Errorf("access denied for user %s: %s", sarUser, reason)
	}

	g.log.Info("access granted", "user", sarUser, "groups", sarGroups, "verb", verb, "resource", resource, "namespace", namespace, "name", name)

	if isAgent {
		c, err := g.factory.ForToken(token)
		if err != nil {
			return nil, fmt.Errorf("building agent client: %w", err)
		}
		return c, nil
	}
	return g.Client, nil
}

// onBehalfOf extracts X-On-Behalf-Of-User and X-On-Behalf-Of-Groups headers.
// Returns empty strings/nil if not present.
func onBehalfOf(req *mcp.CallToolRequest) (user string, groups []string) {
	user = req.Extra.Header.Get(headerOnBehalfOfUser)
	if obg := req.Extra.Header.Get(headerOnBehalfOfGroups); obg != "" {
		groups = strings.Split(obg, ",")
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
	if authHeader == "" {
		return ""
	}
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		return authHeader[7:]
	}
	return authHeader
}

type GetArgs struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (g *Greenhouse) getCRDSchema(ctx context.Context, c client.Client, crdName string) (*mcp.CallToolResult, any, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	key := client.ObjectKey{Name: crdName}
	if err := c.Get(ctx, key, crd); err != nil {
		g.log.Error(err, "failed to get CRD", "name", crdName)
		return nil, nil, kubeErr(err)
	}

	jsonBytes, err := json.MarshalIndent(crd.Spec.Versions, "", "  ")
	if err != nil {
		g.log.Error(err, "failed to marshal CRD schema to JSON")
		return nil, nil, kubeErr(err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonBytes)},
		},
	}, nil, nil
}

// cleanAnnotations removes the kubectl.kubernetes.io/last-applied-configuration
// annotation from the object if it exists.
func cleanAnnotations(obj client.Object) {
	annotations := obj.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		obj.SetAnnotations(annotations)
	}
}

// kubeErr sanitizes a Kubernetes client error: returns a "permission denied" error
// for 403 Forbidden responses so internal API details are not exposed to callers.
func kubeErr(err error) error {
	if apierrors.IsForbidden(err) {
		return errors.New("permission denied")
	}
	return err
}

func MarshalToJSON(obj any) string {
	jsonBytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(jsonBytes)
}
