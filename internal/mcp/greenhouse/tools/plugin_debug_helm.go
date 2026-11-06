// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
)

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

func helmReleaseStatusString(hrs *greenhousev1alpha1.HelmReleaseStatus) string {
	if hrs == nil {
		return ""
	}
	return hrs.Status
}

func fluxDependencyNames(deps []helmv2.DependencyReference) []string {
	var names []string
	for _, dependency := range deps {
		names = append(names, dependency.Name)
	}
	return names
}

func extractHistory(snapshots helmv2.Snapshots, limit int) []releaseSnapshot {
	var history []releaseSnapshot
	for _, snapshot := range snapshots {
		if snapshot == nil {
			continue
		}
		history = append(history, releaseSnapshot{
			ChartVersion: snapshot.ChartVersion,
			AppVersion:   snapshot.AppVersion,
			Status:       snapshot.Status,
			Version:      snapshot.Version,
			LastDeployed: snapshot.LastDeployed.Format(timestampFormat),
		})
	}
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}
	return history
}

func extractUpgradeRemediation(helmRelease *helmv2.HelmRelease) *upgradeRemediationInfo {
	if helmRelease.Spec.Upgrade == nil || helmRelease.Spec.Upgrade.Remediation == nil {
		return nil
	}
	remediation := helmRelease.Spec.Upgrade.Remediation
	strategy := "rollback" // Flux default
	if remediation.Strategy != nil {
		strategy = string(*remediation.Strategy)
	}
	return &upgradeRemediationInfo{
		Retries:              remediation.Retries,
		Strategy:             strategy,
		RemediateLastFailure: remediation.RemediateLastFailure,
	}
}

func extractChartRef(helmRelease *helmv2.HelmRelease) *chartReference {
	if helmRelease.Spec.ChartRef != nil {
		return &chartReference{
			Kind:      helmRelease.Spec.ChartRef.Kind,
			Name:      helmRelease.Spec.ChartRef.Name,
			Namespace: helmRelease.Spec.ChartRef.Namespace,
		}
	}
	if helmRelease.Spec.Chart != nil {
		return &chartReference{
			Kind:      helmRelease.Spec.Chart.Spec.SourceRef.Kind,
			Name:      helmRelease.Spec.Chart.Spec.SourceRef.Name,
			Namespace: helmRelease.Spec.Chart.Spec.SourceRef.Namespace,
		}
	}
	return nil
}

func extractImageReplacements(postRenderers []helmv2.PostRenderer) []imageReplacement {
	var replacements []imageReplacement
	for _, postRenderer := range postRenderers {
		if postRenderer.Kustomize == nil {
			continue
		}
		for _, image := range postRenderer.Kustomize.Images {
			replacements = append(replacements, imageReplacement{
				Original: image.Name,
				Replaced: image.NewName,
				Tag:      image.NewTag,
				Digest:   image.Digest,
			})
		}
	}
	return replacements
}
