// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	greenhouseapis "github.com/cloudoperators/greenhouse/api"
	greenhousemetav1alpha1 "github.com/cloudoperators/greenhouse/api/meta/v1alpha1"
	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterDebugReport struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	KubernetesVersion string            `json:"kubernetesVersion"`
	OnboardingType    string            `json:"onboardingType"`
	BearerTokenExpiry string            `json:"bearerTokenExpiry,omitempty"`
	Nodes             *nodesSummary     `json:"nodes,omitempty"`
	Conditions        []statusCondition `json:"conditions"`
	RecentEvents      []eventEntry      `json:"recentEvents"`
}

type nodesSummary struct {
	Total    int            `json:"total"`
	Ready    int            `json:"ready"`
	NotReady []nodeNotReady `json:"notReady,omitempty"`
}

type nodeNotReady struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type clusterDebugger struct {
	client  client.Client
	log     logr.Logger
	report  clusterDebugReport
	cluster *greenhousev1alpha1.Cluster
}

func newClusterDebugger(c client.Client, log logr.Logger) *clusterDebugger {
	return &clusterDebugger{client: c, log: log}
}

func (d *clusterDebugger) withCluster(ctx context.Context, name, namespace string) *clusterDebugger {
	cluster := &greenhousev1alpha1.Cluster{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cluster); err != nil {
		d.log.Error(err, "failed to get cluster", "name", name, "namespace", namespace)
		d.report = clusterDebugReport{
			Name:       name,
			Namespace:  namespace,
			Conditions: []statusCondition{{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()}},
		}
		return d
	}

	d.cluster = cluster
	ct := "unknown"
	if v, ok := cluster.Annotations[greenhouseapis.ClusterConnectivityAnnotation]; ok {
		ct = v
	}

	report := clusterDebugReport{
		Name:              cluster.Name,
		Namespace:         cluster.Namespace,
		KubernetesVersion: cluster.Status.KubernetesVersion,
		OnboardingType:    ct,
		Conditions:        clusterConditions(cluster.Status.Conditions),
	}

	if !cluster.Status.BearerTokenExpirationTimestamp.IsZero() {
		report.BearerTokenExpiry = cluster.Status.BearerTokenExpirationTimestamp.Format(timestampFormat)
	}

	if cluster.Status.Nodes != nil {
		ns := &nodesSummary{
			Total: cluster.Status.Nodes.Total,
			Ready: cluster.Status.Nodes.Ready,
		}
		for _, n := range cluster.Status.Nodes.NotReady {
			ns.NotReady = append(ns.NotReady, nodeNotReady{Name: n.Name, Message: n.Message})
		}
		report.Nodes = ns
	}

	d.report = report
	return d
}

func (d *clusterDebugger) withEvents(ctx context.Context, name, namespace string) *clusterDebugger {
	eventList := &corev1.EventList{}
	fieldSelector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", name),
		fields.OneTermEqualSelector("involvedObject.kind", "Cluster"),
	)
	if err := d.client.List(ctx, eventList,
		client.InNamespace(namespace),
		client.MatchingFieldsSelector{Selector: fieldSelector},
	); err != nil {
		d.log.V(2).Info("could not list cluster events", "name", name, "error", err)
		return d
	}
	d.report.RecentEvents = recentEvents(eventList.Items, maxRecentEvents)
	return d
}

func (d *clusterDebugger) build() clusterDebugReport {
	return d.report
}

func (g *Greenhouse) DebugCluster(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
	c, err := g.CheckAccess(ctx, req, "get", "clusters", args.Namespace, args.Name)
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	report := newClusterDebugger(c, g.log).
		withCluster(ctx, args.Name, args.Namespace).
		withEvents(ctx, args.Name, args.Namespace).
		build()

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: MarshalToJSON(report)},
		},
	}, nil, nil
}

func clusterConditions(conditions []greenhousemetav1alpha1.Condition) []statusCondition {
	var result []statusCondition
	for _, c := range conditions {
		result = append(result, statusCondition{
			Type:               string(c.Type),
			Status:             string(c.Status),
			Reason:             string(c.Reason),
			Message:            c.Message,
			LastTransitionTime: c.LastTransitionTime.Format(timestampFormat),
		})
	}
	return result
}
