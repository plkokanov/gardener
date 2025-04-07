// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package vpa

import (
	"context"
	"fmt"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/component"
	kubeapiserverconstants "github.com/gardener/gardener/pkg/component/kubernetes/apiserver/constants"
	monitoringutils "github.com/gardener/gardener/pkg/component/observability/monitoring/utils"
	"github.com/gardener/gardener/pkg/utils"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	kubernetesutils "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	"github.com/gardener/gardener/pkg/utils/retry"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
)

const (
	// KubeStateMetricsServiceAccountName is the name of the service account for kube-state-metrics in the shoot cluster.
	KubeStateMetricsServiceAccountName = "kube-state-metrics-" + Label
	// KubeStateMetricsAccessSecretName is the name of the secret containing a token for accessing the shoot cluster.
	KubeStateMetricsAccessSecretName = gardenerutils.SecretNamePrefixShootAccess + KubeStateMetricsServiceAccountName
	// KubeStateMetricsManagedResourceName is the name of the prometheus managed resource for vpa-recommender for the seed.
	KubeStateMetricsManagedResourceName = "vpa-recommender-kube-state-metrics"

	shootKubeStateMetricsManagedResourceName = "shoot-core-" + KubeStateMetricsManagedResourceName
	kubeStateMetricsPort                     = 8080
	kubeStateMetricsContainerName            = "kube-state-metrics"
)

func (v *vpa) kubeStateMetricsManagedResourceName() string {
	if v.values.ClusterType == component.ClusterTypeSeed {
		return KubeStateMetricsManagedResourceName
	}
	return shootKubeStateMetricsManagedResourceName
}

func (v *vpa) waitForKubeStateMetricsToBeUpAndRunning(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, TimeoutWaitForPrometheus)
	defer cancel()

	return retry.Until(timeoutCtx, IntervalWaitForPrometheus, func(ctx context.Context) (done bool, err error) {
		deployment := v.emptyDeployment(v.getKubeStateMetricsDeploymentName())
		if err := v.client.Get(ctx, client.ObjectKeyFromObject(deployment), deployment); err != nil {
			return retry.SevereError(err)
		}

		if err := health.CheckDeployment(deployment); err != nil {
			return retry.MinorError(err)
		}

		return retry.Ok()
	})
}

func (v *vpa) getKubeStateMetricsDeploymentName() string {
	return "kube-state-metrics-" + v.values.Recommender.KubeStateMetrics.Suffix
}

func (v *vpa) kubeStateMetricsForSeed() component.ResourceConfigs {
	var (
		serviceAccount = v.emptyServiceAccount(v.getKubeStateMetricsDeploymentName())
		clusterRole    = v.emptyClusterRole(v.getKubeStateMetricsDeploymentName())
		// TODO(plkokanov): add PDBs
		// ADD VPA if necessary
		clusterRoleBinding = v.emptyClusterRoleBinding(v.getKubeStateMetricsDeploymentName())
		deployment         = v.emptyDeployment(v.getKubeStateMetricsDeploymentName())
	)

	return component.ResourceConfigs{
		{Obj: serviceAccount, Class: component.Runtime, MutateFn: func() { v.reconcileKubeStateMetricsServiceAccount(serviceAccount) }},
		{Obj: clusterRole, Class: component.Runtime, MutateFn: func() { v.reconcileKubeStateMetricsRuntimeClusterRole(clusterRole) }},
		{Obj: clusterRoleBinding, Class: component.Runtime, MutateFn: func() { v.reconcileKubeStateMetricsRuntimeClusterRoleBinding(clusterRoleBinding, clusterRole) }},
		{Obj: deployment, Class: component.Runtime, MutateFn: func() {
			v.reconcileKubeStateMetricsDeployment(deployment, serviceAccount, "", "")
		}},
	}
}

func (v *vpa) kubeStateMetricsForShoot(genericTokenKubeconfigSecretName string, shootAccessSecretName string) component.ResourceConfigs {
	var (
		clusterRole        = v.emptyClusterRole(v.getKubeStateMetricsDeploymentName())
		clusterRoleBinding = v.emptyClusterRoleBinding(v.getKubeStateMetricsDeploymentName())
		deployment         = v.emptyDeployment(v.getKubeStateMetricsDeploymentName())
	)

	return component.ResourceConfigs{
		{Obj: clusterRole, Class: component.Application, MutateFn: func() { v.reconcileKubeStateMetricsClusterRole(clusterRole) }},
		{Obj: clusterRoleBinding, Class: component.Application, MutateFn: func() { v.reconcileKubeStateMetricsClusterRoleBinding(clusterRoleBinding, clusterRole) }},
		{Obj: deployment, Class: component.Runtime, MutateFn: func() {
			v.reconcileKubeStateMetricsDeployment(deployment, nil, genericTokenKubeconfigSecretName, shootAccessSecretName)
		}},
	}
}

func (v *vpa) kubeStateMetricsResourceConfigs() component.ResourceConfigs {
	var (
		kubeStateMetricsScrapeConfig = v.emptyScrapeConfig(kubeStateMetricsScrapeConfigName)
		service                      = v.emptyService(v.getKubeStateMetricsDeploymentName())
	)

	return component.ResourceConfigs{
		{Obj: kubeStateMetricsScrapeConfig, Class: component.Runtime, MutateFn: func() { v.reconcileKubeStateMetricsScrapeConfig(kubeStateMetricsScrapeConfig) }},
		{Obj: service, Class: component.Runtime, MutateFn: func() { v.reconcileKubeStateMetricsService(service) }},
	}
}

func (v *vpa) reconcileKubeStateMetricsServiceAccount(serviceAccount *corev1.ServiceAccount) {
	serviceAccount.Labels = v.getKubeStateMetricsLabels()
	serviceAccount.AutomountServiceAccountToken = ptr.To(false)
}

func (v *vpa) reconcileKubeStateMetricsScrapeConfig(obj *monitoringv1alpha1.ScrapeConfig) {
	obj.Labels = monitoringutils.Labels(v.values.Recommender.Prometheus.Name)
	obj.Spec = monitoringv1alpha1.ScrapeConfigSpec{
		KubernetesSDConfigs: []monitoringv1alpha1.KubernetesSDConfig{{
			// Service is used, because we only care about metric from one kube-state-metrics instance and not multiple
			// in HA setup.
			Role:       monitoringv1alpha1.KubernetesRoleService,
			Namespaces: &monitoringv1alpha1.NamespaceDiscovery{Names: []string{v.namespace}},
		}},
		RelabelConfigs: []monitoringv1.RelabelConfig{
			{
				SourceLabels: []monitoringv1.LabelName{
					"__meta_kubernetes_service_name",
					"__meta_kubernetes_service_port_name",
				},
				Regex:  v.getKubeStateMetricsDeploymentName() + ";" + "metrics",
				Action: "keep",
			},
			{
				Action:      "replace",
				Replacement: ptr.To("kube-state-metrics"),
				TargetLabel: "job",
			},
			{
				TargetLabel: "instance",
				Replacement: ptr.To(v.getKubeStateMetricsDeploymentName()),
			},
		},
		MetricRelabelConfigs: []monitoringv1.RelabelConfig{{
			SourceLabels: []monitoringv1.LabelName{"pod"},
			Regex:        `^.+\.tf-pod.+$`,
			Action:       "drop",
		}},
	}
}

func (v *vpa) reconcileKubeStateMetricsClusterRole(clusterRole *rbacv1.ClusterRole) {
	clusterRole.Labels = getKubeStateMetricsRoleLabels()
	clusterRole.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRole.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"pods",
			},
			Verbs: []string{"list", "watch"},
		},
	}
}

func (v *vpa) reconcileKubeStateMetricsClusterRoleBinding(clusterRoleBinding *rbacv1.ClusterRoleBinding, clusterRole *rbacv1.ClusterRole) {
	clusterRoleBinding.Labels = getKubeStateMetricsRoleLabels()
	clusterRoleBinding.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     clusterRole.Name,
	}
	clusterRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      KubeStateMetricsServiceAccountName,
		Namespace: v.namespaceForApplicationClassResource(),
	}}
}

func (v *vpa) reconcileKubeStateMetricsRuntimeClusterRole(clusterRole *rbacv1.ClusterRole) {
	clusterRole.Labels = getKubeStateMetricsRoleLabels()
	clusterRole.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRole.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{
				"pods",
			},
			Verbs: []string{"list", "watch"},
		},
	}
}

func (v *vpa) reconcileKubeStateMetricsRuntimeClusterRoleBinding(clusterRoleBinding *rbacv1.ClusterRoleBinding, clusterRole *rbacv1.ClusterRole) {
	clusterRoleBinding.Labels = getKubeStateMetricsRoleLabels()
	clusterRoleBinding.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     clusterRole.Name,
	}
	clusterRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      KubeStateMetricsServiceAccountName,
		Namespace: v.namespaceForApplicationClassResource(),
	}}
}

func (v *vpa) reconcileKubeStateMetricsService(service *corev1.Service) {
	service.Labels = v.getKubeStateMetricsLabels()

	metricsPort := networkingv1.NetworkPolicyPort{
		Port:     ptr.To(intstr.FromInt32(kubeStateMetricsPort)),
		Protocol: ptr.To(corev1.ProtocolTCP),
	}

	switch v.values.ClusterType {
	case component.ClusterTypeSeed:
		utilruntime.Must(gardenerutils.InjectNetworkPolicyAnnotationsForGardenScrapeTargets(service, metricsPort))
		utilruntime.Must(gardenerutils.InjectNetworkPolicyAnnotationsForSeedScrapeTargets(service, metricsPort))
	case component.ClusterTypeShoot:
		utilruntime.Must(gardenerutils.InjectNetworkPolicyAnnotationsForScrapeTargets(service, metricsPort))
	}

	service.Spec.Type = corev1.ServiceTypeClusterIP
	service.Spec.Selector = v.getKubeStateMetricsLabels()
	service.Spec.Ports = kubernetesutils.ReconcileServicePorts(service.Spec.Ports, []corev1.ServicePort{
		{
			Name:       "metrics",
			Port:       80,
			TargetPort: intstr.FromInt32(kubeStateMetricsPort),
			Protocol:   corev1.ProtocolTCP,
		},
	}, corev1.ServiceTypeClusterIP)
}

func (v *vpa) reconcileKubeStateMetricsDeployment(deployment *appsv1.Deployment, serviceAccount *corev1.ServiceAccount, genericTokenKubeconfigSecretName string, shootAccessSecretName string) {
	var (
		maxUnavailable = intstr.FromInt32(1)

		deploymentLabels = v.getKubeStateMetricsLabels()
		podLabels        = map[string]string{
			v1beta1constants.LabelNetworkPolicyToDNS: v1beta1constants.LabelNetworkPolicyAllowed,
		}
		args = []string{
			fmt.Sprintf("--port=%d", kubeStateMetricsPort),
			"--telemetry-port=8081",
		}
	)

	if v.values.ClusterType == component.ClusterTypeSeed {
		podLabels = utils.MergeStringMaps(podLabels, deploymentLabels, map[string]string{
			v1beta1constants.LabelNetworkPolicyToRuntimeAPIServer: v1beta1constants.LabelNetworkPolicyAllowed,
		})
	}

	if v.values.ClusterType == component.ClusterTypeShoot {
		podLabels = utils.MergeStringMaps(podLabels, deploymentLabels, map[string]string{
			gardenerutils.NetworkPolicyLabel(v1beta1constants.DeploymentNameKubeAPIServer, kubeapiserverconstants.Port): v1beta1constants.LabelNetworkPolicyAllowed,
		})
	}

	args = append(args,
		"--resources=pods",
		"--metric-allowlist=^kube_pod_labels$",
		"--metric-labels-allowlist=pods=[*]",
	)

	if v.values.ClusterType == component.ClusterTypeShoot {
		args = append(args, "--kubeconfig="+gardenerutils.PathGenericKubeconfig)
	}

	deployment.Labels = deploymentLabels
	deployment.Spec.Replicas = v.values.Recommender.KubeStateMetrics.Replicas
	deployment.Spec.RevisionHistoryLimit = ptr.To[int32](2)
	deployment.Spec.Selector = &metav1.LabelSelector{MatchLabels: v.getKubeStateMetricsLabels()}
	deployment.Spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: &appsv1.RollingUpdateDeployment{
			MaxUnavailable: &maxUnavailable,
		},
	}
	deployment.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: podLabels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:            kubeStateMetricsContainerName,
				Image:           v.values.Recommender.KubeStateMetrics.Image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Args:            args,
				Ports: []corev1.ContainerPort{{
					Name:          "metrics",
					ContainerPort: kubeStateMetricsPort,
					Protocol:      corev1.ProtocolTCP,
				}},
				LivenessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.FromInt32(kubeStateMetricsPort),
						},
					},
					InitialDelaySeconds: 5,
					TimeoutSeconds:      5,
				},
				ReadinessProbe: &corev1.Probe{
					ProbeHandler: corev1.ProbeHandler{
						HTTPGet: &corev1.HTTPGetAction{
							Path: "/healthz",
							Port: intstr.FromInt32(kubeStateMetricsPort),
						},
					},
					InitialDelaySeconds: 5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    3,
					TimeoutSeconds:      5,
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("32Mi"),
					},
				},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: ptr.To(false),
				},
			}},
			PriorityClassName: v.values.Recommender.KubeStateMetrics.PriorityClassName,
		},
	}

	if v.values.ClusterType == component.ClusterTypeSeed {
		deployment.Spec.Template.Spec.ServiceAccountName = serviceAccount.Name
	}

	if v.values.ClusterType == component.ClusterTypeShoot {
		deployment.Spec.Template.Spec.AutomountServiceAccountToken = ptr.To(false)
		utilruntime.Must(gardenerutils.InjectGenericKubeconfig(deployment, genericTokenKubeconfigSecretName, shootAccessSecretName))
	}
}

func getKubeStateMetricsRoleLabels() map[string]string {
	return map[string]string{v1beta1constants.GardenRole: "kube-state-metrics-vpa-recommender"}
}

func (v *vpa) getKubeStateMetricsLabels() map[string]string {
	return map[string]string{
		v1beta1constants.LabelApp:  "kube-state-metrics",
		v1beta1constants.LabelRole: "kube-state-metrics-vpa-recommender",
		"name":                     v.getKubeStateMetricsDeploymentName(),
	}
}
