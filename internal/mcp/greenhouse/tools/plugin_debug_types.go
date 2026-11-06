// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

const (
	centralClusterName    = "greenhouse (central cluster)"
	centralClusterVersion = "n/a"
	unknownVersion        = "unknown"

	maxHistoryLength = 10

	catalogLabel = "greenhouse.sap/catalog"
)

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
