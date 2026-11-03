package tools

import "github.com/modelcontextprotocol/go-sdk/mcp"

func ClusterTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_cluster",
		Title: "Get Greenhouse Cluster",
		Description: `Retrieves a Greenhouse Cluster resource by name and namespace. A Cluster represents a remote Kubernetes cluster onboarded into the Greenhouse platform.
Returns JSON with: metadata, spec (accessMode), status (kubernetesVersion, bearerTokenValidity, statusConditions with Ready/HeadscaleReady states, nodes list).
Use this to check cluster health, connectivity, Kubernetes version, or node inventory. Requires 'name' (cluster name) and 'namespace' (Greenhouse organization).`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func ClusterSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_cluster_schema",
		Title: "Get Greenhouse Cluster Schema",
		Description: `Returns the OpenAPI v3 schema for the Greenhouse Cluster CRD (greenhouse.sap/v1alpha1).
Use this to understand the full structure of a Cluster resource — all spec fields, status fields, and valid enum values — before interpreting get_cluster output or advising on cluster configuration.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func CatalogTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_catalog",
		Title: "Get Catalog resource",
		Description: `Lists all available PluginDefinition and ClusterPluginDefinition resources in a Greenhouse organization. Each entry describes an installable plugin with its helm chart reference, version, description, and UI application details.
Returns JSON array of catalog entries. Use this to discover what plugins are available for installation, compare versions, or find the correct pluginDefinition name before creating or debugging a Plugin.
Requires 'namespace' (Greenhouse organization). Optionally accepts 'name' to filter to a single definition.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func CatalogSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_catalog_schema",
		Title: "Get Greenhouse PluginDefinition Schema",
		Description: `Returns the OpenAPI v3 schema for the PluginDefinition CRD (greenhouse.sap/v1alpha1).
Use this to understand the full structure of a PluginDefinition — helm chart references, UI application specs, option types, and status conditions — before interpreting catalog entries.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func ListPluginsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "list_plugins",
		Title: "List Greenhouse Plugins",
		Description: `Lists all Greenhouse Plugin resources in a namespace. Returns a JSON array with each plugin's metadata, spec (pluginDefinitionRef, clusterName, releaseNamespace, optionValues), and status (helmReleaseStatus, statusConditions).
Use this to get an overview of all deployed plugins in a Greenhouse organization, check their statuses, or find a plugin name before calling get_plugin or debug_plugin. Requires 'namespace' (Greenhouse organization).`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func PluginTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_plugin",
		Title: "Get Greenhouse Plugin",
		Description: `Retrieves a Greenhouse Plugin resource by name and namespace. A Plugin is a deployed instance of a PluginDefinition on a target cluster, backed by a Flux HelmRelease.
Returns JSON with: metadata, spec (pluginDefinitionRef, clusterName, releaseNamespace, optionValues), status (helmReleaseStatus, statusConditions, exposedServices, uiApplication).
Use this to inspect a plugin's configuration, check its deployment status, or review its option values. Requires 'name' and 'namespace'. For deeper diagnostics including cross-resource health checks, use debug_plugin instead.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func PluginSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_plugin_schema",
		Title: "Get Greenhouse Plugin Schema",
		Description: `Returns the OpenAPI v3 schema for the Plugin CRD (greenhouse.sap/v1alpha1).
Use this to understand the full structure of a Plugin resource — spec fields (pluginDefinitionRef, clusterName, releaseNamespace, optionValues), status fields, and valid enum values — before interpreting get_plugin output.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func PluginDefinitionTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_plugin_definition",
		Title: "Get Greenhouse PluginDefinition",
		Description: `Retrieves a Greenhouse PluginDefinition resource by name and namespace. A PluginDefinition describes an installable plugin — its helm chart reference, version, UI application spec, option definitions, and status conditions.
Returns the full JSON resource including labels (e.g. greenhouse.sap/catalog and greenhouse.sap/catalog-source indicating which Catalog and source manages this definition).
Use this to inspect the full details of a PluginDefinition — its helm chart version, available options, or to investigate catalog ownership. Requires 'name' and 'namespace'.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func PluginDefinitionSchemaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "get_plugin_definition_schema",
		Title: "Get Greenhouse PluginDefinition Schema",
		Description: `Returns the OpenAPI v3 schema for the Greenhouse PluginDefinition CRD (greenhouse.sap/v1alpha1).
Use this to understand the full structure of a PluginDefinition — helm chart references, UI application specs, option type definitions, and status conditions — before interpreting get_plugin_definition output or advising on plugin definition configuration.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func DebugPluginTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "debug_plugin",
		Title: "Debug Greenhouse Plugin",
		Description: `Performs a comprehensive cross-resource diagnostic of a Greenhouse Plugin in a single call.
Fetches: (1) the Plugin and its status conditions, (2) the PluginDefinition with helm chart reference (repository, name, version) and labels, (3) the Catalog if the PluginDefinition has a 'greenhouse.sap/catalog' label — including source repositories, git refs, and chart repository overrides, (4) the target Cluster with Kubernetes version and readiness, (5) the Flux HelmRelease with chart reference, image replacements from PostRenderers, reconciliation state, and dependencies, (6) recent Kubernetes events for the HelmRelease.
Returns a structured JSON report with sections: plugin, pluginDefinition, catalog (if applicable), cluster, helmRelease, recentEvents. Each section includes a 'problems' array listing non-ready conditions with type, reason, message, and timestamp.
Key diagnostic fields:
- plugin.suspended: true if the plugin has the 'greenhouse.sap/suspend' annotation — reconciliation and Helm upgrades are paused, and deletion is blocked until the annotation is removed.
- pluginDefinition.helmChart: the authoritative chart source (name, repository, version) from the PluginDefinition spec. Cross-reference with helmRelease.lastAttemptedRevision to detect version mismatches.
- catalog.sources: git repository URLs, resource paths, branch/tag refs, and chart repository overrides that control how PluginDefinitions are resolved. Present only when the PluginDefinition is managed by a Catalog. IMPORTANT: Check for duplicate sources — if the same PluginDefinition resource path (e.g. 'perses/plugindefinition.yaml') appears in multiple catalog sources pinned to different git refs without a distinguishing alias in overrides, the corresponding Flux Kustomizations will fight over the PluginDefinition resource, causing it to flip-flop between chart versions on every reconciliation cycle. This is a common root cause for plugins that repeatedly fail and recover. A high PluginDefinition generation count is a strong indicator of this conflict.
- helmRelease.imageReplacements: container images replaced via Kustomize PostRenderers (applied to ALL rendered manifests including helm hooks). Critical for diagnosing image pull errors — if an image is rejected by an admission policy (e.g. Gatekeeper), check whether it appears here. A missing replacement means the chart uses an image not mapped to an approved registry mirror.
Use this as the first tool when a user reports a plugin is broken, not deploying, or behaving unexpectedly. Requires 'name' and 'namespace'.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}

func DebugClusterTool() *mcp.Tool {
	return &mcp.Tool{
		Name:  "debug_cluster",
		Title: "Debug Greenhouse Cluster",
		Description: `Performs a comprehensive diagnostic of a Greenhouse Cluster in a single call.
Returns: onboarding type (kubeconfig or oidc), Kubernetes version, bearer token expiry, node summary (total/ready/not-ready with per-node messages), all status conditions (type, status, reason, message, lastTransitionTime), and recent Kubernetes events.
Use this as the first tool when a user reports a cluster resource is not ready or a condition is false.`,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
	}
}
