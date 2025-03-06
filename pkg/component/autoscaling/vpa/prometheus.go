package vpa

import (
	"context"
	"fmt"
	"strconv"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringv1alpha1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	secretsutils "github.com/gardener/gardener/pkg/utils/secrets"
)

const (
	// Label is a constant for the label of the cadvisor prometheus instance.
	Label = "vpa-recommender"
	// PrometheusServiceAccountName is the name of the service account in the shoot cluster.
	PrometheusServiceAccountName = "prometheus-" + Label
	// PrometheusAccessSecretName is the name of the secret containing a token for accessing the shoot cluster.
	PrometheusAccessSecretName = gardenerutils.SecretNamePrefixShootAccess + PrometheusServiceAccountName
	// PrometheusManagedResourceName is the name of the prometheus managed resource for vpa-recommender for the seed.
	PrometheusManagedResourceName = "vpa-recommender-prometheus"
	// TimeoutWaitForPrometheus is the timeout to wait until the vpa-recommender prometheus becomes ready.
	TimeoutWaitForPrometheus = 10 * time.Minute
	// IntervalWaitForPrometheus is the interval to check whether the vpa-recommender prometheus has become ready.
	IntervalWaitForPrometheus = 5 * time.Second

	shootPrometheusManagedResourceName = "shoot-core-" + PrometheusManagedResourceName
	prometheusServicePort              = 80
	prometheusServiceTargetPort        = 9090
	prometheusPortName                 = "web"
	cAdvisorScrapeConfigName           = "shoot-vpa-recommender-cadvisor"
	kubeStateMetricsScrapeConfigName   = "shoot-vpa-recommender-kube-state-metrics"
)

func (v *vpa) prometheusManagedResourceName() string {
	if v.values.ClusterType == component.ClusterTypeSeed {
		return PrometheusManagedResourceName
	}
	return shootPrometheusManagedResourceName
}

func (v *vpa) waitForPrometheusToBeUpAndRunning(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, TimeoutWaitForPrometheus)
	defer cancel()

	return retry.Until(timeoutCtx, IntervalWaitForPrometheus, func(ctx context.Context) (done bool, err error) {
		prometheus := v.emptyPrometheus()
		if err := v.client.Get(ctx, client.ObjectKeyFromObject(prometheus), prometheus); err != nil {
			return retry.SevereError(err)
		}

		if err := health.CheckPrometheus(prometheus); err != nil {
			return retry.MinorError(err)
		}

		return retry.Ok()
	})
}

func (v *vpa) prometheusResourceConfigs() component.ResourceConfigs {
	var (
		prometheus               = v.emptyPrometheus()
		prometheusService        = v.emptyPrometheusService()
		cAdvisorScrapeConfig     = v.emptyScrapeConfig(cAdvisorScrapeConfigName)
		clusterRoleTarget        = v.emptyClusterRole(v.prometheusName())
		clusterRoleBindingTarget = v.emptyClusterRoleBinding(v.prometheusName())
		clusterRoleBindingSource = v.emptyClusterRoleBinding(v.prometheusName())
		selfScrapeConfig         = v.emptyScrapeConfig(v.prometheusName())
	)

	return component.ResourceConfigs{
		{Obj: cAdvisorScrapeConfig, Class: component.Runtime, MutateFn: func() { v.reconcileCAdvisorScrapeConfig(cAdvisorScrapeConfig) }},
		{Obj: selfScrapeConfig, Class: component.Runtime, MutateFn: func() { v.reconcileSelfScrapeConfig(selfScrapeConfig) }},
		{Obj: prometheusService, Class: component.Runtime, MutateFn: func() { v.reconcilePrometheusService(prometheusService) }},
		{Obj: v.serviceAccount(), Class: component.Runtime},
		{Obj: prometheus, Class: component.Runtime, MutateFn: func() { v.reconcileRecommenderPrometheus(prometheus) }},
		{Obj: clusterRoleTarget, Class: component.Application, MutateFn: func() {
			v.reconcilePrometheusClusterRoleTarget(clusterRoleTarget)
		}},
		{Obj: clusterRoleBindingTarget, Class: component.Application, MutateFn: func() {
			v.reconcilePrometheusClusterRoleBindingTarget(clusterRoleBindingTarget, clusterRoleTarget)
		}},
		{Obj: clusterRoleBindingSource, Class: component.Runtime, MutateFn: func() {
			v.reconcilePrometheusClusterRoleBindingSource(clusterRoleBindingSource)
		}},
	}
}

func getPrometheusRoleLabel() map[string]string {
	return map[string]string{v1beta1constants.GardenRole: "prometheus-vpa-recommender"}
}

func (v *vpa) reconcilePrometheusClusterRoleTarget(clusterRole *rbacv1.ClusterRole) {
	clusterRole.Labels = getPrometheusRoleLabel()
	clusterRole.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRole.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"nodes", "services", "endpoints", "pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{corev1.GroupName},
			Resources: []string{"nodes/metrics", "pods/log", "nodes/proxy", "services/proxy", "pods/proxy"},
			Verbs:     []string{"get"},
		},
		{
			NonResourceURLs: []string{"/metrics"},
			Verbs:           []string{"get"},
		},
	}
}

func (v *vpa) reconcilePrometheusClusterRoleBindingTarget(clusterRoleBinding *rbacv1.ClusterRoleBinding, clusterRole *rbacv1.ClusterRole) {
	clusterRoleBinding.Labels = getPrometheusRoleLabel()
	clusterRoleBinding.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     clusterRole.Name,
	}
	clusterRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      PrometheusServiceAccountName,
		Namespace: v.namespaceForApplicationClassResource(),
	}}
}

func (v *vpa) reconcilePrometheusClusterRoleBindingSource(clusterRoleBinding *rbacv1.ClusterRoleBinding) {
	clusterRoleBinding.Labels = getPrometheusRoleLabel()
	// TODO(plkokanov): Check if this should be changed to the UID of the shoot's control plane namespace
	// and add an owner reference to the namespace to ensure proper deletion.
	clusterRoleBinding.Name += "-" + v.namespace
	clusterRoleBinding.Annotations = map[string]string{resourcesv1alpha1.DeleteOnInvalidUpdate: "true"}
	clusterRoleBinding.RoleRef = rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     "prometheus",
	}
	clusterRoleBinding.Subjects = []rbacv1.Subject{{
		Kind:      rbacv1.ServiceAccountKind,
		Name:      PrometheusServiceAccountName,
		Namespace: v.namespace,
	}}
}

func (v *vpa) serviceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.prometheusName(),
			Namespace: v.namespace,
			Labels:    v.getRecommenderPrometheusLabels(),
		},
		AutomountServiceAccountToken: ptr.To(false),
	}
}

func (v *vpa) emptyPrometheusService() *corev1.Service {
	return &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: v.prometheusName(), Namespace: v.namespace}}
}

func (v *vpa) reconcilePrometheusService(service *corev1.Service) {
	service.Labels = map[string]string{v1beta1constants.LabelObservabilityApplication: v.prometheusName()}
	metav1.SetMetaDataAnnotation(&service.ObjectMeta, resourcesv1alpha1.NetworkingFromWorldToPorts, fmt.Sprintf(`[{"protocol":"TCP","port":%d}]`, prometheusServiceTargetPort))

	service.Spec.Selector = map[string]string{v1beta1constants.LabelObservabilityApplication: v.prometheusName()}
	desiredPorts := []corev1.ServicePort{{
		Port:       prometheusServicePort,
		TargetPort: intstr.FromInt32(prometheusServiceTargetPort),
		Name:       prometheusPortName,
	}}
	service.Spec.Ports = kubernetesutils.ReconcileServicePorts(service.Spec.Ports, desiredPorts, "")
	metav1.SetMetaDataAnnotation(&service.ObjectMeta, resourcesv1alpha1.NetworkingPodLabelSelectorNamespaceAlias, v1beta1constants.LabelNetworkPolicyShootNamespaceAlias)
	utilruntime.Must(gardenerutils.InjectNetworkPolicyNamespaceSelectors(service, metav1.LabelSelector{MatchLabels: map[string]string{
		corev1.LabelMetadataName: v1beta1constants.GardenNamespace,
	}}))
}

func (v *vpa) emptyScrapeConfig(name string) *monitoringv1alpha1.ScrapeConfig {
	return &monitoringv1alpha1.ScrapeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: v.namespace,
			Labels:    monitoringutils.Labels(v.values.Recommender.Prometheus.Name),
		},
	}
}

func (v *vpa) reconcileCAdvisorScrapeConfig(obj *monitoringv1alpha1.ScrapeConfig) {
	obj.Labels = monitoringutils.Labels(v.values.Recommender.Prometheus.Name)
	obj.Spec = monitoringv1alpha1.ScrapeConfigSpec{
		HonorLabels:     ptr.To(false),
		HonorTimestamps: ptr.To(false),
		Scheme:          ptr.To("HTTPS"),
		Authorization: &monitoringv1.SafeAuthorization{Credentials: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: PrometheusAccessSecretName},
			Key:                  resourcesv1alpha1.DataKeyToken,
		}},
		TLSConfig: &monitoringv1.SafeTLSConfig{CA: monitoringv1.SecretOrConfigMap{Secret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: v.caSecretName},
			Key:                  secretsutils.DataKeyCertificateBundle,
		}}},
		// StaticConfigs: []monitoringv1alpha1.StaticConfig{{
		// 	Targets: []monitoringv1alpha1.Target{"cadvisor:8080"},
		//}},
		KubernetesSDConfigs: []monitoringv1alpha1.KubernetesSDConfig{{
			Role:            monitoringv1alpha1.KubernetesRoleNode,
			APIServer:       ptr.To("https://" + v1beta1constants.DeploymentNameKubeAPIServer + ":" + strconv.Itoa(kubeapiserverconstants.Port)),
			Namespaces:      &monitoringv1alpha1.NamespaceDiscovery{Names: []string{metav1.NamespaceSystem}},
			FollowRedirects: ptr.To(false),
			Authorization: &monitoringv1.SafeAuthorization{Credentials: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: PrometheusAccessSecretName},
				Key:                  resourcesv1alpha1.DataKeyToken,
			}},
			TLSConfig: &monitoringv1.SafeTLSConfig{CA: monitoringv1.SecretOrConfigMap{Secret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: v.caSecretName},
				Key:                  secretsutils.DataKeyCertificateBundle,
			}}},
		}},
		RelabelConfigs: []monitoringv1.RelabelConfig{
			{
				Action:      "replace",
				Replacement: ptr.To("cadvisor"),
				TargetLabel: "job",
			},
			// {
			// 	Action: "labelmap",
			// 	Regex:  `__meta_kubernetes_node_label_(.+)`,
			// },
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
			// {
			// 	TargetLabel: "type",
			// 	Replacement: ptr.To("shoot"),
			// },
		},
		MetricRelabelConfigs: []monitoringv1.RelabelConfig{
			// 	// get system services
			// 	{
			// 		SourceLabels: []monitoringv1.LabelName{"id"},
			// 		Action:       "replace",
			// 		Regex:        `^/system\.slice/(.+)\.service$`,
			// 		TargetLabel:  "systemd_service_name",
			// 	},
			// 	{
			// 		SourceLabels: []monitoringv1.LabelName{"id"},
			// 		Action:       "replace",
			// 		Regex:        `^/system\.slice/(.+)\.service$`,
			// 		Replacement:  ptr.To(`$1`),
			// 		TargetLabel:  "container",
			// 	},
			// monitoringutils.StandardMetricRelabelConfig(
			// 	"container_cpu_cfs_periods_total",
			// 	"container_cpu_cfs_throttled_seconds_total",
			// 	"container_cpu_cfs_throttled_periods_total",
			// 	"container_cpu_usage_seconds_total",
			// 	"container_fs_inodes_total",
			// 	"container_fs_limit_bytes",
			// 	"container_fs_usage_bytes",
			// 	"container_last_seen",
			// 	"container_memory_working_set_bytes",
			// 	"container_network_receive_bytes_total",
			// 	"container_network_transmit_bytes_total",
			// )[0],
			// 	{
			// 		SourceLabels: []monitoringv1.LabelName{"container", "__name__"},
			// 		Action:       "drop",
			// 		// The system container POD is used for networking
			// 		Regex: `POD;(container_cpu_cfs_periods_total|container_cpu_cfs_throttled_seconds_total|container_cpu_cfs_throttled_periods_total|container_cpu_usage_seconds_total|container_fs_inodes_total|container_fs_limit_bytes|container_fs_usage_bytes|container_last_seen|container_memory_working_set_bytes)`,
			// 	},
			// 	{
			// 		SourceLabels: []monitoringv1.LabelName{"__name__", "container", "interface", "id"},
			// 		Action:       "keep",
			// 		Regex:        `container_network.+;;(eth0;/.+|(en.+|tunl0|eth0);/)|.+;.+;.*;.*`,
			// 	},
			// 	{
			// 		SourceLabels: []monitoringv1.LabelName{"__name__", "container", "interface"},
			// 		Action:       "drop",
			// 		Regex:        `container_network.+;POD;(.{5,}|tun0|en.+)`,
			// 	},
			// 	{
			// 		SourceLabels: []monitoringv1.LabelName{"__name__", "id"},
			// 		Regex:        `container_network.+;/`,
			// 		Replacement:  ptr.To("true"),
			// 		TargetLabel:  "host_network",
			// 	},
			// 	{
			// 		Regex:  `^id$`,
			// 		Action: "labeldrop",
			// 	},
		},
	}
}

func (v *vpa) reconcileSelfScrapeConfig(obj *monitoringv1alpha1.ScrapeConfig) {
	obj.Labels = monitoringutils.Labels(v.values.Recommender.Prometheus.Name)
	obj.Spec = monitoringv1alpha1.ScrapeConfigSpec{
		JobName: ptr.To("prometheus"),
		StaticConfigs: []monitoringv1alpha1.StaticConfig{{
			Targets: []monitoringv1alpha1.Target{monitoringv1alpha1.Target("localhost:9090")},
		}},
	}
}

func (v *vpa) getRecommenderPrometheusLabels() map[string]string {
	return map[string]string{
		v1beta1constants.LabelApp:  "prometheus",
		v1beta1constants.LabelRole: "vpa-recommender-history-provider",
		"name":                     v.values.Recommender.Prometheus.Name,
	}
}

func (v *vpa) prometheusName() string {
	return "prometheus-" + v.values.Recommender.Prometheus.Name
}

// TODO(plkokanov): does this prometheus require VPA and PDB?
// TODO(plkokanov): could we reuse the shoot prometheus, but with a suffix?
func (v *vpa) emptyPrometheus() *monitoringv1.Prometheus {
	return &monitoringv1.Prometheus{
		ObjectMeta: metav1.ObjectMeta{
			Name:      v.values.Recommender.Prometheus.Name,
			Namespace: v.namespace,
			Labels:    v.getRecommenderPrometheusLabels(),
		},
	}
}

func (v *vpa) reconcileRecommenderPrometheus(obj *monitoringv1.Prometheus) {
	obj.Spec = monitoringv1.PrometheusSpec{
		RetentionSize:      v.values.Recommender.Prometheus.RetentionSize,
		EvaluationInterval: "1m",
		CommonPrometheusFields: monitoringv1.CommonPrometheusFields{
			ScrapeInterval: "1m",
			ScrapeTimeout:  v.values.Recommender.Prometheus.ScrapeTimeout,
			ReloadStrategy: ptr.To(monitoringv1.HTTPReloadStrategyType),
			ExternalLabels: v.values.Recommender.Prometheus.ExternalLabels,

			PodMetadata: &monitoringv1.EmbeddedObjectMetadata{
				Labels: utils.MergeStringMaps(map[string]string{
					v1beta1constants.LabelNetworkPolicyToDNS:                                                     v1beta1constants.LabelNetworkPolicyAllowed,
					v1beta1constants.LabelNetworkPolicyToRuntimeAPIServer:                                        v1beta1constants.LabelNetworkPolicyAllowed,
					v1beta1constants.LabelObservabilityApplication:                                               v.prometheusName(),
					"networking.resources.gardener.cloud/to-" + v1beta1constants.LabelNetworkPolicyScrapeTargets: v1beta1constants.LabelNetworkPolicyAllowed,
				}),
			},
			PriorityClassName: v.values.Recommender.PriorityClassName,
			Replicas:          v.values.Recommender.Prometheus.Replicas,
			Shards:            ptr.To[int32](1),
			Image:             &v.values.Recommender.Prometheus.Image,
			ImagePullPolicy:   corev1.PullIfNotPresent,
			Version:           v.values.Recommender.Prometheus.Version,
			Resources: corev1.ResourceRequirements{
				Requests: ptr.Deref(v.values.Recommender.Prometheus.ResourceRequests, corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("300m"),
					corev1.ResourceMemory: resource.MustParse("1000Mi"),
				}),
			},
			ServiceAccountName: v.prometheusName(),
			SecurityContext:    &corev1.PodSecurityContext{RunAsUser: ptr.To[int64](0)},
			Storage: &monitoringv1.StorageSpec{
				VolumeClaimTemplate: monitoringv1.EmbeddedPersistentVolumeClaim{
					EmbeddedObjectMetadata: monitoringv1.EmbeddedObjectMetadata{Name: "prometheus-db"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources:   corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: v.values.Recommender.Prometheus.StorageCapacity}},
					},
				},
			},

			ScrapeConfigSelector: &metav1.LabelSelector{MatchLabels: monitoringutils.Labels(v.values.Recommender.Prometheus.Name)},
			Web: &monitoringv1.PrometheusWebSpec{
				MaxConnections: ptr.To[int32](1024),
			},
		},
		RuleSelector:          &metav1.LabelSelector{MatchLabels: monitoringutils.Labels(v.values.Recommender.Prometheus.Name)},
		RuleNamespaceSelector: &metav1.LabelSelector{},
	}

	if v.values.Recommender.Prometheus.Retention != nil {
		obj.Spec.Retention = *v.values.Recommender.Prometheus.Retention
	}
}
