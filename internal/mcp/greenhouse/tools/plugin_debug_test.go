// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/goccy/go-json"

	greenhousemetav1alpha1 "github.com/cloudoperators/greenhouse/api/meta/v1alpha1"
	greenhousev1alpha1 "github.com/cloudoperators/greenhouse/api/v1alpha1"
	"github.com/cloudoperators/greenhouse/pkg/lifecycle"
	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/pkg/apis/kustomize"
	fluxmeta "github.com/fluxcd/pkg/apis/meta"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudoperators/greenhouse-mcp/internal/mocks"
)

func TestDebugPlugin(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		args    GetArgs
		setup   func(mc *mocks.MockClient)
		wantErr string
		check   func(t *testing.T, report debugReport)
	}{
		{
			name:    "missing token returns error",
			token:   "",
			args:    GetArgs{Name: "my-plugin", Namespace: "my-org"},
			setup:   func(_ *mocks.MockClient) {},
			wantErr: "no bearer token found in request",
		},
		{
			name:  "access denied returns error",
			token: "valid-token",
			args:  GetArgs{Name: "my-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", []string{"developers"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(denyAccess()).
					Return(nil).Once()
			},
			wantErr: "access denied for user alice",
		},
		{
			name:  "plugin not found produces fetch error in report",
			token: "valid-token",
			args:  GetArgs{Name: "missing-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()
				// Plugin Get fails
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Return(errors.New("plugins.greenhouse.sap \"missing-plugin\" not found")).Once()
				// HelmRelease Get (still called even when plugin not found)
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Return(errors.New("not found")).Once()
				// Events List
				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				assert.Equal(t, "missing-plugin", report.Plugin.Name)
				require.Len(t, report.Plugin.Conditions, 1)
				assert.Equal(t, conditionTypeFetch, report.Plugin.Conditions[0].Type)
				assert.Contains(t, report.Plugin.Conditions[0].Message, "not found")
				// PluginDefinition should not be fetched when plugin is nil
				assert.False(t, report.PluginDefinition.Found)
			},
		},
		{
			name:  "full successful debug report",
			token: "valid-token",
			args:  GetArgs{Name: "thanos", Namespace: "sci-demo"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("bob", []string{"admins"})).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				// Plugin
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{
							Name:      "thanos",
							Namespace: "sci-demo",
						}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{
								Name: "kube-monitoring",
								Kind: "PluginDefinition",
							},
							ClusterName:      "cluster-a",
							ReleaseNamespace: "monitoring",
						}
						p.Status.HelmReleaseStatus = &greenhousev1alpha1.HelmReleaseStatus{Status: "deployed"}
					}).
					Return(nil).Once()

				// PluginDefinition
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{
							Name:      "kube-monitoring",
							Namespace: "sci-demo",
							Labels: map[string]string{
								"greenhouse.sap/catalog": "my-catalog",
							},
						}
						pd.Spec = greenhousev1alpha1.PluginDefinitionSpec{
							HelmChart: &greenhousev1alpha1.HelmChartReference{
								Name:       "kube-monitoring",
								Repository: "oci://registry.example.com/charts",
								Version:    "1.5.0",
							},
						}
					}).
					Return(nil).Once()

				// Catalog (triggered by label)
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Catalog"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						cat := obj.(*greenhousev1alpha1.Catalog)
						branch := "main"
						cat.ObjectMeta = metav1.ObjectMeta{
							Name:      "my-catalog",
							Namespace: "sci-demo",
						}
						cat.Spec = greenhousev1alpha1.CatalogSpec{
							Sources: []greenhousev1alpha1.CatalogSource{
								{
									Repository: "https://github.com/org/charts",
									Resources:  []string{"plugins/kube-monitoring.yaml"},
									Ref:        &greenhousev1alpha1.GitRef{Branch: &branch},
								},
							},
						}
					}).
					Return(nil).Once()

				// Cluster
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Cluster"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						c := obj.(*greenhousev1alpha1.Cluster)
						c.ObjectMeta = metav1.ObjectMeta{
							Name:      "cluster-a",
							Namespace: "sci-demo",
						}
						c.Status.KubernetesVersion = "1.29.4"
					}).
					Return(nil).Once()

				// HelmRelease
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						hr := obj.(*helmv2.HelmRelease)
						hr.ObjectMeta = metav1.ObjectMeta{
							Name:      "thanos",
							Namespace: "sci-demo",
						}
						hr.Spec.ChartRef = &helmv2.CrossNamespaceSourceReference{
							Kind: "HelmChart",
							Name: "kube-monitoring-1.5.0",
						}
						hr.Spec.DependsOn = []helmv2.DependencyReference{
							{Name: "cert-manager"},
						}
						hr.Spec.PostRenderers = []helmv2.PostRenderer{
							{
								Kustomize: &helmv2.Kustomize{
									Images: []kustomize.Image{
										{Name: "quay.io/thanos/thanos", NewName: "registry.example.com/thanos", NewTag: "v0.35.0"},
									},
								},
							},
						}
						hr.Status.Conditions = []metav1.Condition{
							{
								Type:               fluxmeta.ReadyCondition,
								Status:             metav1.ConditionTrue,
								LastTransitionTime: metav1.Now(),
							},
						}
						hr.Status.LastAttemptedRevision = "1.5.0"
						hr.Status.LastAttemptedReleaseAction = "upgrade"
						hr.Status.History = helmv2.Snapshots{
							{
								ChartVersion: "1.5.0",
								AppVersion:   "0.35.0",
								Status:       "deployed",
								Version:      3,
								LastDeployed: metav1.NewTime(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)),
							},
						}
					}).
					Return(nil).Once()

				// Events
				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Run(func(_ context.Context, list client.ObjectList, _ ...client.ListOption) {
						el := list.(*corev1.EventList)
						el.Items = []corev1.Event{
							{
								Type:          "Normal",
								Reason:        "UpgradeSucceeded",
								Message:       "Helm upgrade succeeded",
								LastTimestamp: metav1.NewTime(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)),
							},
						}
					}).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				// Plugin section
				assert.Equal(t, "thanos", report.Plugin.Name)
				assert.Equal(t, "sci-demo", report.Plugin.Namespace)
				assert.Equal(t, "kube-monitoring", report.Plugin.PluginDefinition)
				assert.Equal(t, "cluster-a", report.Plugin.ClusterName)
				assert.Equal(t, "deployed", report.Plugin.HelmReleaseStatus)
				assert.False(t, report.Plugin.Suspended)
				assert.Empty(t, report.Plugin.Conditions)

				// PluginDefinition section
				assert.True(t, report.PluginDefinition.Found)
				assert.Equal(t, "kube-monitoring", report.PluginDefinition.Name)
				require.NotNil(t, report.PluginDefinition.HelmChart)
				assert.Equal(t, "1.5.0", report.PluginDefinition.HelmChart.Version)
				assert.Equal(t, "oci://registry.example.com/charts", report.PluginDefinition.HelmChart.Repository)

				// Catalog section
				require.NotNil(t, report.Catalog)
				assert.True(t, report.Catalog.Found)
				assert.Equal(t, "my-catalog", report.Catalog.Name)
				require.Len(t, report.Catalog.Sources, 1)
				assert.Equal(t, "main", report.Catalog.Sources[0].Branch)
				assert.Equal(t, "https://github.com/org/charts", report.Catalog.Sources[0].Repository)

				// Cluster section
				assert.Equal(t, "cluster-a", report.Cluster.Name)
				assert.Equal(t, "1.29.4", report.Cluster.KubernetesVersion)
				assert.Empty(t, report.Cluster.Conditions)

				// HelmRelease section
				assert.True(t, report.HelmRelease.Found)
				assert.True(t, report.HelmRelease.Ready)
				require.NotNil(t, report.HelmRelease.ChartRef)
				assert.Equal(t, "kube-monitoring-1.5.0", report.HelmRelease.ChartRef.Name)
				assert.Equal(t, []string{"cert-manager"}, report.HelmRelease.Dependencies)
				require.Len(t, report.HelmRelease.ImageReplacements, 1)
				assert.Equal(t, "quay.io/thanos/thanos", report.HelmRelease.ImageReplacements[0].Original)
				assert.Equal(t, "registry.example.com/thanos", report.HelmRelease.ImageReplacements[0].Replaced)
				assert.Equal(t, "v0.35.0", report.HelmRelease.ImageReplacements[0].Tag)
				assert.Equal(t, "1.5.0", report.HelmRelease.LastAttemptedRevision)
				assert.Equal(t, "upgrade", report.HelmRelease.LastAttemptedReleaseAction)
				require.Len(t, report.HelmRelease.History, 1)
				assert.Equal(t, "deployed", report.HelmRelease.History[0].Status)
				assert.Equal(t, 3, report.HelmRelease.History[0].Version)
				require.Len(t, report.HelmRelease.Conditions, 1)
				assert.Equal(t, fluxmeta.ReadyCondition, report.HelmRelease.Conditions[0].Type)
				assert.Equal(t, string(metav1.ConditionTrue), report.HelmRelease.Conditions[0].Status)

				// Events section
				require.Len(t, report.RecentEvents, 1)
				assert.Equal(t, "UpgradeSucceeded", report.RecentEvents[0].Reason)
			},
		},
		{
			name:  "suspended plugin is detected",
			token: "valid-token",
			args:  GetArgs{Name: "suspended-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{
							Name:      "suspended-plugin",
							Namespace: "my-org",
							Annotations: map[string]string{
								lifecycle.SuspendAnnotation: "true",
							},
						}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{Name: "alerts"},
							ClusterName:         "cluster-b",
						}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{Name: "alerts", Namespace: "my-org"}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Cluster"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						c := obj.(*greenhousev1alpha1.Cluster)
						c.ObjectMeta = metav1.ObjectMeta{Name: "cluster-b", Namespace: "my-org"}
						c.Status.KubernetesVersion = "1.28.0"
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Return(errors.New("not found")).Once()

				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				assert.True(t, report.Plugin.Suspended)
				assert.Equal(t, "suspended-plugin", report.Plugin.Name)
			},
		},
		{
			name:  "empty cluster name maps to central cluster",
			token: "valid-token",
			args:  GetArgs{Name: "central-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{Name: "central-plugin", Namespace: "my-org"}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{Name: "perses"},
							ClusterName:         "",
						}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{Name: "perses", Namespace: "my-org"}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Return(errors.New("not found")).Once()

				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				assert.Equal(t, centralClusterName, report.Cluster.Name)
				assert.Equal(t, centralClusterVersion, report.Cluster.KubernetesVersion)
			},
		},
		{
			name:  "plugin with non-ready conditions reports problems",
			token: "valid-token",
			args:  GetArgs{Name: "broken-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{Name: "broken-plugin", Namespace: "my-org"}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{Name: "alerts"},
							ClusterName:         "cluster-c",
						}
						p.Status.Conditions = []greenhousemetav1alpha1.Condition{
							{
								Type:               greenhousemetav1alpha1.ReadyCondition,
								Status:             metav1.ConditionFalse,
								Reason:             "HelmReleaseFailed",
								Message:            "upgrade retries exhausted",
								LastTransitionTime: metav1.NewTime(time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)),
							},
						}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{Name: "alerts", Namespace: "my-org"}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Cluster"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						c := obj.(*greenhousev1alpha1.Cluster)
						c.ObjectMeta = metav1.ObjectMeta{Name: "cluster-c", Namespace: "my-org"}
						c.Status.KubernetesVersion = "1.28.0"
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						hr := obj.(*helmv2.HelmRelease)
						hr.ObjectMeta = metav1.ObjectMeta{Name: "broken-plugin", Namespace: "my-org"}
						hr.Status.Conditions = []metav1.Condition{
							{
								Type:               fluxmeta.ReadyCondition,
								Status:             metav1.ConditionFalse,
								Reason:             "UpgradeFailed",
								Message:            "chart values validation failed",
								LastTransitionTime: metav1.NewTime(time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)),
							},
						}
						hr.Status.UpgradeFailures = 5
						hr.Status.LastAttemptedReleaseAction = "upgrade"
					}).
					Return(nil).Once()

				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				require.Len(t, report.Plugin.Conditions, 1)
				assert.Equal(t, string(greenhousemetav1alpha1.ReadyCondition), report.Plugin.Conditions[0].Type)
				assert.Equal(t, "upgrade retries exhausted", report.Plugin.Conditions[0].Message)

				assert.True(t, report.HelmRelease.Found)
				assert.False(t, report.HelmRelease.Ready)
				assert.Equal(t, int64(5), report.HelmRelease.UpgradeFailures)
				require.Len(t, report.HelmRelease.Conditions, 1)
				assert.Equal(t, "UpgradeFailed", report.HelmRelease.Conditions[0].Reason)
			},
		},
		{
			name:  "cluster fetch failure reports problem",
			token: "valid-token",
			args:  GetArgs{Name: "orphan-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{Name: "orphan-plugin", Namespace: "my-org"}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{Name: "alerts"},
							ClusterName:         "deleted-cluster",
						}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{Name: "alerts", Namespace: "my-org"}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Cluster"), mock.Anything).
					Return(errors.New("clusters.greenhouse.sap \"deleted-cluster\" not found")).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Return(errors.New("not found")).Once()

				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				assert.Equal(t, "deleted-cluster", report.Cluster.Name)
				assert.Equal(t, unknownVersion, report.Cluster.KubernetesVersion)
				require.Len(t, report.Cluster.Conditions, 1)
				assert.Equal(t, conditionTypeFetch, report.Cluster.Conditions[0].Type)
			},
		},
		{
			name:  "helm release with upgrade remediation",
			token: "valid-token",
			args:  GetArgs{Name: "remediated-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{Name: "remediated-plugin", Namespace: "my-org"}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{Name: "alerts"},
							ClusterName:         "cluster-d",
						}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{Name: "alerts", Namespace: "my-org"}
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Cluster"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						c := obj.(*greenhousev1alpha1.Cluster)
						c.ObjectMeta = metav1.ObjectMeta{Name: "cluster-d", Namespace: "my-org"}
						c.Status.KubernetesVersion = "1.30.0"
					}).
					Return(nil).Once()

				remediateLastFailure := true
				uninstall := helmv2.RemediationStrategy("uninstall")
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						hr := obj.(*helmv2.HelmRelease)
						hr.ObjectMeta = metav1.ObjectMeta{Name: "remediated-plugin", Namespace: "my-org"}
						hr.Spec.Upgrade = &helmv2.Upgrade{
							Remediation: &helmv2.UpgradeRemediation{
								Retries:              3,
								Strategy:             &uninstall,
								RemediateLastFailure: &remediateLastFailure,
							},
						}
						hr.Status.Conditions = []metav1.Condition{
							{
								Type:               fluxmeta.ReadyCondition,
								Status:             metav1.ConditionTrue,
								LastTransitionTime: metav1.Now(),
							},
						}
					}).
					Return(nil).Once()

				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				require.NotNil(t, report.HelmRelease.UpgradeRemediation)
				assert.Equal(t, 3, report.HelmRelease.UpgradeRemediation.Retries)
				assert.Equal(t, "uninstall", report.HelmRelease.UpgradeRemediation.Strategy)
				require.NotNil(t, report.HelmRelease.UpgradeRemediation.RemediateLastFailure)
				assert.True(t, *report.HelmRelease.UpgradeRemediation.RemediateLastFailure)
			},
		},
		{
			name:  "catalog fetch failure reports problem",
			token: "valid-token",
			args:  GetArgs{Name: "cat-fail-plugin", Namespace: "my-org"},
			setup: func(mc *mocks.MockClient) {
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.TokenReview"), mock.Anything).
					Run(authenticateUser("alice", nil)).
					Return(nil).Once()
				mc.EXPECT().
					Create(mock.Anything, mock.AnythingOfType("*v1.SubjectAccessReview"), mock.Anything).
					Run(allowAccess()).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Plugin"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						p := obj.(*greenhousev1alpha1.Plugin)
						p.ObjectMeta = metav1.ObjectMeta{Name: "cat-fail-plugin", Namespace: "my-org"}
						p.Spec = greenhousev1alpha1.PluginSpec{
							PluginDefinitionRef: greenhousev1alpha1.PluginDefinitionReference{Name: "alerts"},
							ClusterName:         "cluster-e",
						}
					}).
					Return(nil).Once()

				// PluginDefinition with catalog label
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.PluginDefinition"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						pd := obj.(*greenhousev1alpha1.PluginDefinition)
						pd.ObjectMeta = metav1.ObjectMeta{
							Name:      "alerts",
							Namespace: "my-org",
							Labels:    map[string]string{catalogLabel: "missing-catalog"},
						}
					}).
					Return(nil).Once()

				// Catalog fetch fails
				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Catalog"), mock.Anything).
					Return(errors.New("catalogs.greenhouse.sap \"missing-catalog\" not found")).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v1alpha1.Cluster"), mock.Anything).
					Run(func(_ context.Context, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) {
						c := obj.(*greenhousev1alpha1.Cluster)
						c.ObjectMeta = metav1.ObjectMeta{Name: "cluster-e", Namespace: "my-org"}
						c.Status.KubernetesVersion = "1.28.0"
					}).
					Return(nil).Once()

				mc.EXPECT().
					Get(mock.Anything, mock.Anything, mock.AnythingOfType("*v2.HelmRelease"), mock.Anything).
					Return(errors.New("not found")).Once()

				mc.EXPECT().
					List(mock.Anything, mock.AnythingOfType("*v1.EventList"), mock.Anything, mock.Anything).
					Return(nil).Once()
			},
			check: func(t *testing.T, report debugReport) {
				require.NotNil(t, report.Catalog)
				assert.False(t, report.Catalog.Found)
				assert.Equal(t, "missing-catalog", report.Catalog.Name)
				require.Len(t, report.Catalog.Conditions, 1)
				assert.Contains(t, report.Catalog.Conditions[0].Message, "not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, mc := newTestGreenhouse(t)
			tt.setup(mc)

			req := reqWithToken(tt.token)
			result, _, err := g.DebugPlugin(context.Background(), req, tt.args)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Len(t, result.Content, 1)
			text := result.Content[0].(*mcp.TextContent).Text

			var report debugReport
			require.NoError(t, json.Unmarshal([]byte(text), &report))

			if tt.check != nil {
				tt.check(t, report)
			}
		})
	}
}

func TestExtractGreenhouseConditions(t *testing.T) {
	conditions := []greenhousemetav1alpha1.Condition{
		{
			Type:               greenhousemetav1alpha1.ReadyCondition,
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		{
			Type:               "HelmChartReady",
			Status:             metav1.ConditionFalse,
			Reason:             "ChartNotFound",
			Message:            "chart not found in repository",
			LastTransitionTime: metav1.NewTime(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)),
		},
	}

	result := extractGreenhouseConditions(conditions)
	require.Len(t, result, 2)
	assert.Equal(t, string(greenhousemetav1alpha1.ReadyCondition), result[0].Type)
	assert.Equal(t, "HelmChartReady", result[1].Type)
	assert.Equal(t, "ChartNotFound", result[1].Reason)
	assert.Equal(t, "chart not found in repository", result[1].Message)
	assert.Equal(t, "2025-02-01T00:00:00Z", result[1].LastTransitionTime)
}

func TestExtractFluxConditions(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:               fluxmeta.ReadyCondition,
			Status:             metav1.ConditionFalse,
			Reason:             "UpgradeFailed",
			Message:            "timed out",
			LastTransitionTime: metav1.NewTime(time.Date(2025, 3, 15, 8, 30, 0, 0, time.UTC)),
		},
		{
			Type:               "Released",
			Status:             metav1.ConditionTrue,
			LastTransitionTime: metav1.NewTime(time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)),
		},
	}

	result := extractFluxConditions(conditions)
	require.Len(t, result, 2)
	assert.Equal(t, fluxmeta.ReadyCondition, result[0].Type)
	assert.Equal(t, "UpgradeFailed", result[0].Reason)
	assert.Equal(t, "Released", result[1].Type)
}

func TestExtractImageReplacements(t *testing.T) {
	postRenderers := []helmv2.PostRenderer{
		{Kustomize: nil},
		{
			Kustomize: &helmv2.Kustomize{
				Images: []kustomize.Image{
					{Name: "docker.io/grafana/grafana", NewName: "mirror.example.com/grafana", NewTag: "10.0.0"},
					{Name: "quay.io/prometheus/prometheus", NewName: "mirror.example.com/prometheus", Digest: "sha256:abc123"},
				},
			},
		},
	}

	replacements := extractImageReplacements(postRenderers)
	require.Len(t, replacements, 2)
	assert.Equal(t, "docker.io/grafana/grafana", replacements[0].Original)
	assert.Equal(t, "mirror.example.com/grafana", replacements[0].Replaced)
	assert.Equal(t, "10.0.0", replacements[0].Tag)
	assert.Empty(t, replacements[0].Digest)
	assert.Equal(t, "sha256:abc123", replacements[1].Digest)
}

func TestExtractHistory(t *testing.T) {
	snapshots := helmv2.Snapshots{
		{
			ChartVersion: "1.0.0",
			AppVersion:   "0.1.0",
			Status:       "superseded",
			Version:      1,
			LastDeployed: metav1.NewTime(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		},
		{
			ChartVersion: "1.1.0",
			AppVersion:   "0.2.0",
			Status:       "deployed",
			Version:      2,
			LastDeployed: metav1.NewTime(time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)),
		},
		nil,
	}

	history := extractHistory(snapshots, 10)
	require.Len(t, history, 2)
	assert.Equal(t, "1.0.0", history[0].ChartVersion)
	assert.Equal(t, "1.1.0", history[1].ChartVersion)
	assert.Equal(t, 2, history[1].Version)

	// Test limit
	limited := extractHistory(snapshots, 1)
	require.Len(t, limited, 1)
}

func TestRecentEvents(t *testing.T) {
	events := []corev1.Event{
		{
			Type:          "Normal",
			Reason:        "UpgradeSucceeded",
			Message:       "upgrade ok",
			LastTimestamp: metav1.NewTime(time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)),
		},
		{
			Type:          "Warning",
			Reason:        "UpgradeFailed",
			Message:       "timed out",
			LastTimestamp: metav1.NewTime(time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)),
		},
		{
			Type:          "Normal",
			Reason:        "InstallSucceeded",
			Message:       "install ok",
			LastTimestamp: metav1.NewTime(time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)),
		},
	}

	entries := recentEvents(events, 2)
	require.Len(t, entries, 2)
	// Should be sorted by most recent first
	assert.Equal(t, "UpgradeFailed", entries[0].Reason)
	assert.Equal(t, "UpgradeSucceeded", entries[1].Reason)
}

func TestFluxDependencyNames(t *testing.T) {
	deps := []helmv2.DependencyReference{
		{Name: "cert-manager"},
		{Name: "ingress-nginx"},
	}
	names := fluxDependencyNames(deps)
	assert.Equal(t, []string{"cert-manager", "ingress-nginx"}, names)
}

func TestIsFluxHelmReleaseReady(t *testing.T) {
	ready := []metav1.Condition{
		{Type: fluxmeta.ReadyCondition, Status: metav1.ConditionTrue},
	}
	notReady := []metav1.Condition{
		{Type: fluxmeta.ReadyCondition, Status: metav1.ConditionFalse},
	}
	empty := []metav1.Condition{}

	assert.True(t, isFluxHelmReleaseReady(ready))
	assert.False(t, isFluxHelmReleaseReady(notReady))
	assert.False(t, isFluxHelmReleaseReady(empty))
}

func TestHelmReleaseStatusString(t *testing.T) {
	assert.Equal(t, "", helmReleaseStatusString(nil))
	assert.Equal(t, "deployed", helmReleaseStatusString(&greenhousev1alpha1.HelmReleaseStatus{Status: "deployed"}))
}

func TestExtractUpgradeRemediation(t *testing.T) {
	// nil upgrade
	hr := &helmv2.HelmRelease{}
	assert.Nil(t, extractUpgradeRemediation(hr))

	// nil remediation
	hr.Spec.Upgrade = &helmv2.Upgrade{}
	assert.Nil(t, extractUpgradeRemediation(hr))

	// with remediation
	rollback := helmv2.RemediationStrategy("rollback")
	hr.Spec.Upgrade.Remediation = &helmv2.UpgradeRemediation{
		Retries:  5,
		Strategy: &rollback,
	}
	info := extractUpgradeRemediation(hr)
	require.NotNil(t, info)
	assert.Equal(t, 5, info.Retries)
	assert.Equal(t, "rollback", info.Strategy)
	assert.Nil(t, info.RemediateLastFailure)
}

func TestExtractChartRef(t *testing.T) {
	// ChartRef takes precedence
	hr := &helmv2.HelmRelease{}
	hr.Spec.ChartRef = &helmv2.CrossNamespaceSourceReference{
		Kind:      "HelmChart",
		Name:      "my-chart",
		Namespace: "flux-system",
	}
	ref := extractChartRef(hr)
	require.NotNil(t, ref)
	assert.Equal(t, "HelmChart", ref.Kind)
	assert.Equal(t, "my-chart", ref.Name)
	assert.Equal(t, "flux-system", ref.Namespace)

	// Fallback to Chart.Spec.SourceRef
	hr.Spec.ChartRef = nil
	hr.Spec.Chart = &helmv2.HelmChartTemplate{
		Spec: helmv2.HelmChartTemplateSpec{
			SourceRef: helmv2.CrossNamespaceObjectReference{
				Kind:      "HelmRepository",
				Name:      "bitnami",
				Namespace: "flux-system",
			},
		},
	}
	ref = extractChartRef(hr)
	require.NotNil(t, ref)
	assert.Equal(t, "HelmRepository", ref.Kind)
	assert.Equal(t, "bitnami", ref.Name)

	// Neither
	hr.Spec.Chart = nil
	assert.Nil(t, extractChartRef(hr))
}
