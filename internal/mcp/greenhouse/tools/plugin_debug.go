package tools

import (
	"context"
	"sort"

	greenhousemetav1alpha1 "github.com/cloudoperators/greenhouse/api/meta/v1alpha1"
	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/cloudoperators/greenhouse/pkg/lifecycle"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	fluxmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/go-logr/logr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	conditionTypeFetch    = "FetchError"
	conditionTypeNotFound = "NotFound"
	statusError           = "Error"

	centralClusterName    = "greenhouse (central cluster)"
	centralClusterVersion = "n/a"
	unknownVersion        = "unknown"

	timestampFormat  = "2006-01-02T15:04:05Z"
	maxRecentEvents  = 10
	maxHistoryLength = 10

	catalogLabel = "greenhouse.sap/catalog"
)

type statusCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type pluginReport struct {
	Name                 string            `json:"name"`
	Namespace            string            `json:"namespace"`
	PluginDefinition     string            `json:"pluginDefinition"`
	PluginDefinitionKind string            `json:"pluginDefinitionKind"`
	ClusterName          string            `json:"clusterName"`
	ReleaseNamespace     string            `json:"releaseNamespace"`
	HelmReleaseStatus    string            `json:"helmReleaseStatus"`
	Suspended            bool              `json:"suspended"`
	Conditions           []statusCondition `json:"conditions"`
}

type clusterReport struct {
	Name              string            `json:"name"`
	KubernetesVersion string            `json:"kubernetesVersion"`
	Conditions        []statusCondition `json:"conditions"`
}

type imageReplacement struct {
	Original string `json:"original"`
	Replaced string `json:"replaced"`
	Tag      string `json:"tag,omitempty"`
	Digest   string `json:"digest,omitempty"`
}

type releaseSnapshot struct {
	ChartVersion string `json:"chartVersion"`
	AppVersion   string `json:"appVersion,omitempty"`
	Status       string `json:"status"`
	Version      int    `json:"version"`
	LastDeployed string `json:"lastDeployed"`
}

type chartReference struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type upgradeRemediationInfo struct {
	Retries              int    `json:"retries"`
	Strategy             string `json:"strategy"`
	RemediateLastFailure *bool  `json:"remediateLastFailure,omitempty"`
}

type helmReleaseReport struct {
	Found                      bool                    `json:"found"`
	Ready                      bool                    `json:"ready"`
	ChartRef                   *chartReference         `json:"chartRef,omitempty"`
	Dependencies               []string                `json:"dependencies,omitempty"`
	ImageReplacements          []imageReplacement      `json:"imageReplacements,omitempty"`
	LastAttemptedRevision      string                  `json:"lastAttemptedRevision,omitempty"`
	LastAttemptedReleaseAction string                  `json:"lastAttemptedReleaseAction,omitempty"`
	UpgradeFailures            int64                   `json:"upgradeFailures,omitempty"`
	InstallFailures            int64                   `json:"installFailures,omitempty"`
	UpgradeRemediation         *upgradeRemediationInfo `json:"upgradeRemediation,omitempty"`
	History                    []releaseSnapshot       `json:"history,omitempty"`
	Conditions                 []statusCondition       `json:"conditions"`
}

type eventEntry struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

type helmChartInfo struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	Version    string `json:"version"`
}

type pluginDefinitionReport struct {
	Found      bool              `json:"found"`
	Name       string            `json:"name"`
	HelmChart  *helmChartInfo    `json:"helmChart,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Conditions []statusCondition `json:"conditions"`
}

type catalogSourceOverride struct {
	Name       string `json:"name"`
	Alias      string `json:"alias,omitempty"`
	Repository string `json:"repository"`
}

type catalogSourceInfo struct {
	Repository string                  `json:"repository"`
	Resources  []string                `json:"resources,omitempty"`
	Branch     string                  `json:"branch,omitempty"`
	Tag        string                  `json:"tag,omitempty"`
	Overrides  []catalogSourceOverride `json:"overrides,omitempty"`
}

type catalogReport struct {
	Found      bool                `json:"found"`
	Name       string              `json:"name"`
	Sources    []catalogSourceInfo `json:"sources,omitempty"`
	Conditions []statusCondition   `json:"conditions"`
}

type debugReport struct {
	Plugin           pluginReport           `json:"plugin"`
	PluginDefinition pluginDefinitionReport `json:"pluginDefinition"`
	Catalog          *catalogReport         `json:"catalog,omitempty"`
	Cluster          clusterReport          `json:"cluster"`
	HelmRelease      helmReleaseReport      `json:"helmRelease"`
	RecentEvents     []eventEntry           `json:"recentEvents"`
}

func extractGreenhouseConditions(conditions []greenhousemetav1alpha1.Condition) []statusCondition {
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

func extractFluxConditions(conditions []metav1.Condition) []statusCondition {
	var result []statusCondition
	for _, c := range conditions {
		result = append(result, statusCondition{
			Type:               c.Type,
			Status:             string(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			LastTransitionTime: c.LastTransitionTime.Format(timestampFormat),
		})
	}
	return result
}

func helmReleaseStatusString(hrs *greenhousev1alpha1.HelmReleaseStatus) string {
	if hrs == nil {
		return ""
	}
	return hrs.Status
}

func isFluxHelmReleaseReady(conditions []metav1.Condition) bool {
	readyCondition := meta.FindStatusCondition(conditions, fluxmeta.ReadyCondition)
	return readyCondition != nil && readyCondition.Status == metav1.ConditionTrue
}

func fluxDependencyNames(deps []helmv2.DependencyReference) []string {
	var names []string
	for _, d := range deps {
		names = append(names, d.Name)
	}
	return names
}

func recentEvents(events []corev1.Event, limit int) []eventEntry {
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTimestamp.After(events[j].LastTimestamp.Time)
	})

	var entries []eventEntry
	for i, e := range events {
		if i >= limit {
			break
		}
		entries = append(entries, eventEntry{
			Type:    e.Type,
			Reason:  e.Reason,
			Message: e.Message,
			Time:    e.LastTimestamp.Format(timestampFormat),
		})
	}
	return entries
}

type debugger struct {
	client client.Client
	log    logr.Logger
	report debugReport
	plugin *greenhousev1alpha1.Plugin
}

func newDebugger(c client.Client, log logr.Logger) *debugger {
	return &debugger{client: c, log: log}
}

func (d *debugger) withPlugin(ctx context.Context, name, namespace string) *debugger {
	plugin := &greenhousev1alpha1.Plugin{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, plugin); err != nil {
		d.log.Error(err, "failed to get Plugin", "name", name, "namespace", namespace)
		d.report.Plugin = pluginReport{
			Name: name, Namespace: namespace,
			Conditions: []statusCondition{{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()}},
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
	if err := d.client.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: d.plugin.Namespace}, cluster); err != nil {
		d.log.Error(err, "failed to get Cluster", "name", clusterName, "namespace", d.plugin.Namespace)
		d.report.Cluster = clusterReport{Name: clusterName, KubernetesVersion: unknownVersion,
			Conditions: []statusCondition{{Type: conditionTypeFetch, Status: statusError, Message: err.Error()}}}
		return d
	}

	d.report.Cluster = clusterReport{
		Name:              clusterName,
		KubernetesVersion: cluster.Status.KubernetesVersion,
		Conditions:        extractGreenhouseConditions(cluster.Status.Conditions),
	}
	return d
}

func extractHistory(snapshots helmv2.Snapshots, limit int) []releaseSnapshot {
	var history []releaseSnapshot
	for _, s := range snapshots {
		if s == nil {
			continue
		}
		history = append(history, releaseSnapshot{
			ChartVersion: s.ChartVersion,
			AppVersion:   s.AppVersion,
			Status:       s.Status,
			Version:      s.Version,
			LastDeployed: s.LastDeployed.Format(timestampFormat),
		})
	}
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}
	return history
}

func extractUpgradeRemediation(hr *helmv2.HelmRelease) *upgradeRemediationInfo {
	if hr.Spec.Upgrade == nil || hr.Spec.Upgrade.Remediation == nil {
		return nil
	}
	rem := hr.Spec.Upgrade.Remediation
	strategy := "rollback" // Flux default
	if rem.Strategy != nil {
		strategy = string(*rem.Strategy)
	}
	return &upgradeRemediationInfo{
		Retries:              rem.Retries,
		Strategy:             strategy,
		RemediateLastFailure: rem.RemediateLastFailure,
	}
}

func extractChartRef(hr *helmv2.HelmRelease) *chartReference {
	if hr.Spec.ChartRef != nil {
		return &chartReference{
			Kind:      hr.Spec.ChartRef.Kind,
			Name:      hr.Spec.ChartRef.Name,
			Namespace: hr.Spec.ChartRef.Namespace,
		}
	}
	if hr.Spec.Chart != nil {
		return &chartReference{
			Kind:      hr.Spec.Chart.Spec.SourceRef.Kind,
			Name:      hr.Spec.Chart.Spec.SourceRef.Name,
			Namespace: hr.Spec.Chart.Spec.SourceRef.Namespace,
		}
	}
	return nil
}

func extractImageReplacements(postRenderers []helmv2.PostRenderer) []imageReplacement {
	var replacements []imageReplacement
	for _, pr := range postRenderers {
		if pr.Kustomize == nil {
			continue
		}
		for _, img := range pr.Kustomize.Images {
			r := imageReplacement{
				Original: img.Name,
				Replaced: img.NewName,
				Tag:      img.NewTag,
				Digest:   img.Digest,
			}
			replacements = append(replacements, r)
		}
	}
	return replacements
}

func (d *debugger) withHelmRelease(ctx context.Context, name, namespace string) *debugger {
	hr := &helmv2.HelmRelease{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, hr); err != nil {
		d.log.V(2).Info("HelmRelease not found", "name", name, "namespace", namespace, "error", err)
		d.report.HelmRelease = helmReleaseReport{
			Found:      false,
			Conditions: []statusCondition{{Type: conditionTypeNotFound, Status: statusError, Message: kubeErr(err).Error()}},
		}
		return d
	}

	d.report.HelmRelease = helmReleaseReport{
		Found:                      true,
		Ready:                      isFluxHelmReleaseReady(hr.Status.Conditions),
		ChartRef:                   extractChartRef(hr),
		Dependencies:               fluxDependencyNames(hr.Spec.DependsOn),
		ImageReplacements:          extractImageReplacements(hr.Spec.PostRenderers),
		LastAttemptedRevision:      hr.Status.LastAttemptedRevision,
		LastAttemptedReleaseAction: string(hr.Status.LastAttemptedReleaseAction),
		UpgradeFailures:            hr.Status.UpgradeFailures,
		InstallFailures:            hr.Status.InstallFailures,
		UpgradeRemediation:         extractUpgradeRemediation(hr),
		History:                    extractHistory(hr.Status.History, maxHistoryLength),
		Conditions:                 extractFluxConditions(hr.Status.Conditions),
	}
	return d
}

func (d *debugger) withPluginDefinition(ctx context.Context) *debugger {
	if d.plugin == nil {
		return d
	}

	pdName := d.plugin.Spec.PluginDefinitionRef.Name
	namespace := d.plugin.Namespace

	pd := &greenhousev1alpha1.PluginDefinition{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: pdName, Namespace: namespace}, pd); err != nil {
		d.log.Error(err, "failed to get PluginDefinition", "name", pdName, "namespace", namespace)
		d.report.PluginDefinition = pluginDefinitionReport{
			Found: false, Name: pdName,
			Conditions: []statusCondition{{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()}},
		}
		return d
	}

	var chart *helmChartInfo
	if pd.Spec.HelmChart != nil {
		chart = &helmChartInfo{
			Name:       pd.Spec.HelmChart.Name,
			Repository: pd.Spec.HelmChart.Repository,
			Version:    pd.Spec.HelmChart.Version,
		}
	}

	d.report.PluginDefinition = pluginDefinitionReport{
		Found:      true,
		Name:       pd.Name,
		HelmChart:  chart,
		Labels:     pd.Labels,
		Conditions: extractGreenhouseConditions(pd.Status.Conditions),
	}

	// If the PluginDefinition has a catalog label, fetch the Catalog too
	if catalogName, ok := pd.Labels[catalogLabel]; ok {
		d.withCatalog(ctx, catalogName, namespace)
	}

	return d
}

func (d *debugger) withCatalog(ctx context.Context, name, namespace string) {
	cat := &greenhousev1alpha1.Catalog{}
	if err := d.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, cat); err != nil {
		d.log.Error(err, "failed to get Catalog", "name", name, "namespace", namespace)
		d.report.Catalog = &catalogReport{
			Found: false, Name: name,
			Conditions: []statusCondition{{Type: conditionTypeFetch, Status: statusError, Message: kubeErr(err).Error()}},
		}
		return
	}

	var sources []catalogSourceInfo
	for _, src := range cat.Spec.Sources {
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
		for _, o := range src.Overrides {
			info.Overrides = append(info.Overrides, catalogSourceOverride{
				Name:       o.Name,
				Alias:      o.Alias,
				Repository: o.Repository,
			})
		}
		sources = append(sources, info)
	}

	d.report.Catalog = &catalogReport{
		Found:      true,
		Name:       cat.Name,
		Sources:    sources,
		Conditions: extractGreenhouseConditions(cat.Status.Conditions),
	}
}

func (d *debugger) withEvents(ctx context.Context, name, namespace string) *debugger {
	eventList := &corev1.EventList{}
	fieldSelector := fields.AndSelectors(
		fields.OneTermEqualSelector("involvedObject.name", name),
		fields.OneTermEqualSelector("involvedObject.kind", helmv2.HelmReleaseKind),
	)
	if err := d.client.List(ctx, eventList,
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
	c, err := g.CheckAccess(ctx, req, "get", "plugins", args.Namespace, args.Name)
	if err != nil {
		return nil, nil, kubeErr(err)
	}

	report := newDebugger(c, g.log).
		withPlugin(ctx, args.Name, args.Namespace).
		withPluginDefinition(ctx).
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
