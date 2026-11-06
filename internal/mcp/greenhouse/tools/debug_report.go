// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

// Shared constants used by both plugin and cluster debug builders.
const (
	conditionTypeFetch    = "FetchError"
	conditionTypeNotFound = "NotFound"
	statusError           = "Error"

	timestampFormat = "2006-01-02T15:04:05Z"
	maxRecentEvents = 10
)

// statusCondition is a normalised representation of a Kubernetes condition
// used across all debug report types.
type statusCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

// eventEntry holds a single Kubernetes event in a compact form for debug reports.
type eventEntry struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	Time    string `json:"time"`
}
