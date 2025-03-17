// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"time"

	"github.com/Masterminds/semver/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/imagevector"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/component/autoscaling/vpa"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	imagevectorutils "github.com/gardener/gardener/pkg/utils/imagevector"
	secretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager"
)

// NewVerticalPodAutoscaler instantiates a new `vertical-pod-autoscaler` component.
func NewVerticalPodAutoscaler(
	c client.Client,
	gardenNamespaceName string,
	runtimeVersion *semver.Version,
	secretsManager secretsmanager.Interface,
	enabled bool,
	secretNameServerCA string,
	priorityClassNameAdmissionController string,
	priorityClassNameRecommender string,
	priorityClassNameUpdater string,
) (
	component.DeployWaiter,
	error,
) {
	imageAdmissionController, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpaAdmissionController, imagevectorutils.TargetVersion(runtimeVersion.String()))
	if err != nil {
		return nil, err
	}

	imageRecommender, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpaRecommender, imagevectorutils.TargetVersion(runtimeVersion.String()))
	if err != nil {
		return nil, err
	}

	imageUpdater, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpaUpdater, imagevectorutils.TargetVersion(runtimeVersion.String()))
	if err != nil {
		return nil, err
	}

	imagePrometheus, err := imagevector.Containers().FindImage(imagevector.ContainerImageNamePrometheus, imagevectorutils.RuntimeVersion(runtimeVersion.String()))
	if err != nil {
		return nil, err
	}

	imageKubeStateMetrics, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameKubeStateMetrics, imagevectorutils.RuntimeVersion(runtimeVersion.String()))
	if err != nil {
		return nil, err
	}

	return vpa.New(
		c,
		gardenNamespaceName,
		secretsManager,
		vpa.Values{
			ClusterType:              component.ClusterTypeSeed,
			Enabled:                  enabled,
			SecretNameServerCA:       secretNameServerCA,
			RuntimeKubernetesVersion: runtimeVersion,
			AdmissionController: vpa.ValuesAdmissionController{
				Image:             imageAdmissionController.String(),
				PriorityClassName: priorityClassNameAdmissionController,
			},
			Recommender: vpa.ValuesRecommender{
				Image:                        imageRecommender.String(),
				PriorityClassName:            priorityClassNameRecommender,
				RecommendationMarginFraction: ptr.To(float64(0.05)),
				Prometheus: vpa.ValuesPrometheus{
					Name:              "vpa-recommender",
					Image:             imagePrometheus.String(),
					PriorityClassName: v1beta1constants.PriorityClassNameShootControlPlane500,
					StorageCapacity:   resource.MustParse("2Gi"),
					Replicas:          ptr.To[int32](1),
					RetentionSize:     "1GB",
					ScrapeTimeout:     "50s", // This is intentionally smaller than the scrape interval of 1m.
					AdditionalPodLabels: map[string]string{
						gardenerutils.NetworkPolicyLabel("prometheus-garden", 9090): v1beta1constants.LabelNetworkPolicyAllowed,
					},
					Version: ptr.Deref(imagePrometheus.Version, "v0.0.0"),
					ResourceRequests: &corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("400M"),
					},
					VPAMinAllowed: &corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("400Mi")},
				},
				KubeStateMetrics: vpa.ValuesKubeStateMetrics{
					Suffix:            "vpa-recommender",
					Image:             imageKubeStateMetrics.String(),
					PriorityClassName: v1beta1constants.PriorityClassNameShootControlPlane500,
					Replicas:          ptr.To[int32](1),
				},
			},
			Updater: vpa.ValuesUpdater{
				EvictionTolerance:      ptr.To(float64(1.0)),
				EvictAfterOOMThreshold: &metav1.Duration{Duration: 48 * time.Hour},
				Image:                  imageUpdater.String(),
				PriorityClassName:      priorityClassNameUpdater,
			},
		},
	), nil
}
