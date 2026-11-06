// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"

	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/cloudoperators/greenhouse/pkg/lifecycle"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type debugger struct {
	kubeClient  client.Client
	log         logr.Logger
	report      debugReport
	plugin      *greenhousev1alpha1.Plugin
	catalogName string
}

func newDebugger(kubeClient client.Client, log logr.Logger) *debugger {
	return &debugger{kubeClient: kubeClient, log: log}
}

func (d *debugger) withPlugin(ctx context.Context, name, namespace string) *debugger {
	plugin := &greenhousev1alpha1.Plugin{}
	if err := d.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, plugin); err != nil {
		d.log.Error(err, "failed to get Plugin", "name", name, "namespace", namespace)
		d.report.Plugin = pluginReport{
			Name:      name,
			Namespace: namespace,
			Conditions: []statusCondition{
				{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()},
			},
		}
		return d
	}

	d.plugin = plugin
	_, suspended := plugin.Annotations[lifecycle.SuspendAnnotation]
	d.report.Plugin = pluginReport{
		Name:                 plugin.Name,
		Namespace:            plugin.Namespace,
		PluginDefinition:     plugin.Spec.PluginDefinitionRef.Name,
		PluginDefinitionKind: plugin.Spec.PluginDefinitionRef.Kind,
		ClusterName:          plugin.Spec.ClusterName,
		ReleaseNamespace:     plugin.Spec.ReleaseNamespace,
		HelmReleaseStatus:    helmReleaseStatusString(plugin.Status.HelmReleaseStatus),
		Suspended:            suspended,
		Conditions:           extractGreenhouseConditions(plugin.Status.Conditions),
	}
	return d
}

func (d *debugger) withCluster(ctx context.Context) *debugger {
	if d.plugin == nil {
		return d
	}

	clusterName := d.plugin.Spec.ClusterName
	if clusterName == "" {
		d.report.Cluster = clusterReport{Name: centralClusterName, KubernetesVersion: centralClusterVersion}
		return d
	}

	cluster := &greenhousev1alpha1.Cluster{}
	if err := d.kubeClient.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: d.plugin.Namespace}, cluster); err != nil {
		d.log.Error(err, "failed to get Cluster", "name", clusterName, "namespace", d.plugin.Namespace)
		d.report.Cluster = clusterReport{
			Name:              clusterName,
			KubernetesVersion: unknownVersion,
			Conditions:        []statusCondition{{Type: conditionTypeFetch, Status: statusError, Message: err.Error()}},
		}
		return d
	}

	d.report.Cluster = clusterReport{
		Name:              clusterName,
		KubernetesVersion: cluster.Status.KubernetesVersion,
		Conditions:        extractGreenhouseConditions(cluster.Status.Conditions),
	}
	return d
}

func (d *debugger) withPluginDefinition(ctx context.Context) *debugger {
	if d.plugin == nil {
		return d
	}

	pdName := d.plugin.Spec.PluginDefinitionRef.Name
	namespace := d.plugin.Namespace

	pluginDefinition := &greenhousev1alpha1.PluginDefinition{}
	if err := d.kubeClient.Get(ctx, client.ObjectKey{Name: pdName, Namespace: namespace}, pluginDefinition); err != nil {
		d.log.Error(err, "failed to get PluginDefinition", "name", pdName, "namespace", namespace)
		d.report.PluginDefinition = pluginDefinitionReport{
			Found: false,
			Name:  pdName,
			Conditions: []statusCondition{
				{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()},
			},
		}
		return d
	}

	var chart *helmChartInfo
	if pluginDefinition.Spec.HelmChart != nil {
		chart = &helmChartInfo{
			Name:       pluginDefinition.Spec.HelmChart.Name,
			Repository: pluginDefinition.Spec.HelmChart.Repository,
			Version:    pluginDefinition.Spec.HelmChart.Version,
		}
	}

	d.report.PluginDefinition = pluginDefinitionReport{
		Found:      true,
		Name:       pluginDefinition.Name,
		HelmChart:  chart,
		Labels:     pluginDefinition.Labels,
		Conditions: extractGreenhouseConditions(pluginDefinition.Status.Conditions),
	}

	if catalogName, ok := pluginDefinition.Labels[catalogLabel]; ok {
		d.catalogName = catalogName
	}

	return d
}

// withCatalog fetches the Catalog identified by withPluginDefinition.
// No-op when no catalog label was found on the PluginDefinition.
func (d *debugger) withCatalog(ctx context.Context) *debugger {
	if d.catalogName == "" || d.plugin == nil {
		return d
	}

	namespace := d.plugin.Namespace
	catalog := &greenhousev1alpha1.Catalog{}
	if err := d.kubeClient.Get(ctx, client.ObjectKey{Name: d.catalogName, Namespace: namespace}, catalog); err != nil {
		d.log.Error(err, "failed to get Catalog", "name", d.catalogName, "namespace", namespace)
		d.report.Catalog = &catalogReport{
			Found: false,
			Name:  d.catalogName,
			Conditions: []statusCondition{
				{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()},
			},
		}
		return d
	}

	var sources []catalogSourceInfo
	for _, src := range catalog.Spec.Sources {
		info := catalogSourceInfo{
			Repository: src.Repository,
			Resources:  src.Resources,
		}
		if src.Ref != nil {
			if src.Ref.Branch != nil {
				info.Branch = *src.Ref.Branch
			}
			if src.Ref.Tag != nil {
				info.Tag = *src.Ref.Tag
			}
		}
		for _, override := range src.Overrides {
			info.Overrides = append(info.Overrides, catalogSourceOverride{
				Name:       override.Name,
				Alias:      override.Alias,
				Repository: override.Repository,
			})
		}
		sources = append(sources, info)
	}

	d.report.Catalog = &catalogReport{
		Found:      true,
		Name:       catalog.Name,
		Sources:    sources,
		Conditions: extractGreenhouseConditions(catalog.Status.Conditions),
	}
	return d
}

func (d *debugger) withHelmRelease(ctx context.Context, name, namespace string) *debugger {
	helmRelease := &helmv2.HelmRelease{}
	if err := d.kubeClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, helmRelease); err != nil {
		d.log.V(2).Info("HelmRelease not found", "name", name, "namespace", namespace, "error", err)
		d.report.HelmRelease = helmReleaseReport{
			Found:      false,
			Conditions: []statusCondition{{Type: conditionTypeNotFound, Status: statusError, Message: kubeErr(err).Error()}},
		}
		return d
	}

	d.report.HelmRelease = helmReleaseReport{
		Found:                      true,
		Ready:                      isFluxHelmReleaseReady(helmRelease.Status.Conditions),
		ChartRef:                   extractChartRef(helmRelease),
		Dependencies:               fluxDependencyNames(helmRelease.Spec.DependsOn),
		ImageReplacements:          extractImageReplacements(helmRelease.Spec.PostRenderers),
		LastAttemptedRevision:      helmRelease.Status.LastAttemptedRevision,
		LastAttemptedReleaseAction: string(helmRelease.Status.LastAttemptedReleaseAction),
		UpgradeFailures:            helmRelease.Status.UpgradeFailures,
		InstallFailures:            helmRelease.Status.InstallFailures,
		UpgradeRemediation:         extractUpgradeRemediation(helmRelease),
		History:                    extractHistory(helmRelease.Status.History, maxHistoryLength),
		Conditions:                 extractFluxConditions(helmRelease.Status.Conditions),
	}
	return d
}

func (d *debugger) withEvents(ctx context.Context, name, namespace string) *debugger {
	eventList := &corev1.EventList{}
	fieldSelector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", name),
		fields.OneTermEqualSelector("involvedObject.kind", helmv2.HelmReleaseKind),
	)
	if err := d.kubeClient.List(ctx, eventList,
		client.InNamespace(namespace),
		client.MatchingFieldsSelector{Selector: fieldSelector},
	); err != nil {
		d.log.Error(err, "failed to list events", "namespace", namespace, "name", name)
		return d
	}
	d.report.RecentEvents = recentEvents(eventList.Items, maxRecentEvents)
	return d
}

func (d *debugger) build() debugReport {
	return d.report
}

func (g *Greenhouse) DebugPlugin(ctx context.Context, req *mcp.CallToolRequest, args GetArgs) (*mcp.CallToolResult, any, error) {
	kubeClient, err := g.CheckAccess(ctx, req, "get", "plugins", args.Namespace, args.Name)
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	report := newDebugger(kubeClient, g.log).
		withPlugin(ctx, args.Name, args.Namespace).
		withPluginDefinition(ctx).
		withCatalog(ctx).
		withCluster(ctx).
		withHelmRelease(ctx, args.Name, args.Namespace).
		withEvents(ctx, args.Name, args.Namespace).
		build()

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: MarshalToJSON(report)},
		},
	}, nil, nil
}
