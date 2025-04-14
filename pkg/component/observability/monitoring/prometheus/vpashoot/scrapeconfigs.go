// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package vpashoot

import (
	"strconv"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	kubeapiserverconstants "github.com/gardener/gardener/pkg/component/kubernetes/apiserver/constants"
	monitoringutils "github.com/gardener/gardener/pkg/component/observability/monitoring/utils"
	secretsutils "github.com/gardener/gardener/pkg/utils/secrets"
)

// CentralScrapeConfigs returns the central ScrapeConfig resources for the shoot prometheus.
func CentralScrapeConfigs(clusterCASecretName string) []*monitoringv1alpha1.ScrapeConfig {
	return []*monitoringv1alpha1.ScrapeConfig{{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cadvisor",
		},
		Spec: monitoringv1alpha1.ScrapeConfigSpec{
			HonorLabels:     ptr.To(false),
			HonorTimestamps: ptr.To(false),
			Scheme:          ptr.To("HTTPS"),
			Authorization: &monitoringv1.SafeAuthorization{Credentials: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: AccessSecretName},
				Key:                  resourcesv1alpha1.DataKeyToken,
			}},
			TLSConfig: &monitoringv1.SafeTLSConfig{CA: monitoringv1.SecretOrConfigMap{Secret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: clusterCASecretName},
				Key:                  secretsutils.DataKeyCertificateBundle,
			}}},
			KubernetesSDConfigs: []monitoringv1alpha1.KubernetesSDConfig{{
				Role:            monitoringv1alpha1.KubernetesRoleNode,
				APIServer:       ptr.To("https://" + v1beta1constants.DeploymentNameKubeAPIServer + ":" + strconv.Itoa(kubeapiserverconstants.Port)),
				Namespaces:      &monitoringv1alpha1.NamespaceDiscovery{Names: []string{metav1.NamespaceSystem}},
				FollowRedirects: ptr.To(false),
				Authorization: &monitoringv1.SafeAuthorization{Credentials: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: AccessSecretName},
					Key:                  resourcesv1alpha1.DataKeyToken,
				}},
				TLSConfig: &monitoringv1.SafeTLSConfig{CA: monitoringv1.SecretOrConfigMap{Secret: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: clusterCASecretName},
					Key:                  secretsutils.DataKeyCertificateBundle,
				}}},
			}},
			RelabelConfigs: []monitoringv1.RelabelConfig{
				{
					Action:      "replace",
					Replacement: ptr.To("cadvisor"),
					TargetLabel: "job",
				},
				{
					Action: "labelmap",
					Regex:  `__meta_kubernetes_node_label_(.+)`,
				},
				{
					TargetLabel: "__address__",
					Replacement: ptr.To(v1beta1constants.DeploymentNameKubeAPIServer + ":" + strconv.Itoa(kubeapiserverconstants.Port)),
				},
				{
					SourceLabels: []monitoringv1.LabelName{"__meta_kubernetes_node_name"},
					Regex:        `(.+)`,
					Replacement:  ptr.To(`/api/v1/nodes/${1}/proxy/metrics/cadvisor`),
					TargetLabel:  "__metrics_path__",
				},
				{
					TargetLabel: "type",
					Replacement: ptr.To("shoot"),
				},
			},
			MetricRelabelConfigs: []monitoringv1.RelabelConfig{
				monitoringutils.StandardMetricRelabelConfig(
					"container_cpu_usage_seconds_total",
					"container_memory_working_set_bytes",
				)[0],
				{
					SourceLabels: []monitoringv1.LabelName{"container", "__name__"},
					Action:       "drop",
					// The system container POD is used for networking
					Regex: `POD;(container_cpu_usage_seconds_total|container_memory_working_set_bytes)`,
				},
				{
					SourceLabels: []monitoringv1.LabelName{"__name__", "container", "interface"},
					Action:       "drop",
					Regex:        `container_network.+;POD;(.{5,}|tun0|en.+)`,
				},
				{
					Regex:  `^id$`,
					Action: "labeldrop",
				},
			},
		},
	}}
}
