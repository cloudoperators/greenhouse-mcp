// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"sort"

	greenhousemetav1alpha1 "github.com/cloudoperators/greenhouse/api/meta/v1alpha1"
	fluxmeta "github.com/fluxcd/pkg/apis/meta"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// extractGreenhouseConditions converts Greenhouse-typed conditions to the
// normalised statusCondition used in debug reports.
func extractGreenhouseConditions(conditions []greenhousemetav1alpha1.Condition) []statusCondition {
	var result []statusCondition
	for _, condition := range conditions {
		result = append(result, statusCondition{
			Type:               string(condition.Type),
			Status:             string(condition.Status),
			Reason:             string(condition.Reason),
			Message:            condition.Message,
			LastTransitionTime: condition.LastTransitionTime.Format(timestampFormat),
		})
	}
	return result
}

// extractFluxConditions converts Flux metav1.Conditions to the normalised
// statusCondition used in debug reports.
func extractFluxConditions(conditions []metav1.Condition) []statusCondition {
	var result []statusCondition
	for _, condition := range conditions {
		result = append(result, statusCondition{
			Type:               condition.Type,
			Status:             string(condition.Status),
			Reason:             condition.Reason,
			Message:            condition.Message,
			LastTransitionTime: condition.LastTransitionTime.Format(timestampFormat),
		})
	}
	return result
}

// isFluxHelmReleaseReady returns true when the Ready condition is True.
func isFluxHelmReleaseReady(conditions []metav1.Condition) bool {
	readyCondition := meta.FindStatusCondition(conditions, fluxmeta.ReadyCondition)
	return readyCondition != nil && readyCondition.Status == metav1.ConditionTrue
}

// recentEvents sorts events by most-recent-first and returns up to limit entries.
func recentEvents(events []corev1.Event, limit int) []eventEntry {
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastTimestamp.After(events[j].LastTimestamp.Time)
	})

	var entries []eventEntry
	for i, event := range events {
		if i >= limit {
			break
		}
		entries = append(entries, eventEntry{
			Type:    event.Type,
			Reason:  event.Reason,
			Message: event.Message,
			Time:    event.LastTimestamp.Format(timestampFormat),
		})
	}
	return entries
}
