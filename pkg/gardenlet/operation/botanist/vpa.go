// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package botanist

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	"github.com/gardener/gardener/imagevector"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/component/autoscaling/vpa"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
	imagevectorutils "github.com/gardener/gardener/pkg/utils/imagevector"
)

// DefaultVerticalPodAutoscaler returns a deployer for the Kubernetes Vertical Pod Autoscaler.
func (b *Botanist) DefaultVerticalPodAutoscaler() (vpa.Interface, error) {
	imageAdmissionController, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpaAdmissionController, imagevectorutils.RuntimeVersion(b.SeedVersion()), imagevectorutils.TargetVersion(b.ShootVersion()))
	if err != nil {
		return nil, err
	}

	imageRecommender, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpaRecommender, imagevectorutils.RuntimeVersion(b.SeedVersion()), imagevectorutils.TargetVersion(b.ShootVersion()))
	if err != nil {
		return nil, err
	}

	imageUpdater, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameVpaUpdater, imagevectorutils.RuntimeVersion(b.SeedVersion()), imagevectorutils.TargetVersion(b.ShootVersion()))
	if err != nil {
		return nil, err
	}

	imagePrometheus, err := imagevector.Containers().FindImage(imagevector.ContainerImageNamePrometheus, imagevectorutils.RuntimeVersion(b.SeedVersion()))
	if err != nil {
		return nil, err
	}

	imageKubeStateMetrics, err := imagevector.Containers().FindImage(imagevector.ContainerImageNameKubeStateMetrics, imagevectorutils.RuntimeVersion(b.SeedVersion()))
	if err != nil {
		return nil, err
	}

	var (
		valuesAdmissionController = vpa.ValuesAdmissionController{
			Image:                       imageAdmissionController.String(),
			PriorityClassName:           v1beta1constants.PriorityClassNameShootControlPlane200,
			Replicas:                    ptr.To(b.Shoot.GetReplicas(1)),
			TopologyAwareRoutingEnabled: b.Shoot.TopologyAwareRoutingEnabled,
		}
		valuesPrometheus = vpa.ValuesPrometheus{
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
		}
		valuesKubeStateMetrics = vpa.ValuesKubeStateMetrics{
			Suffix:            "vpa-recommender",
			Image:             imageKubeStateMetrics.String(),
			PriorityClassName: v1beta1constants.PriorityClassNameShootControlPlane500,
			Replicas:          ptr.To[int32](1),
		}
		valuesRecommender = vpa.ValuesRecommender{
			Image:             imageRecommender.String(),
			PriorityClassName: v1beta1constants.PriorityClassNameShootControlPlane200,
			Replicas:          ptr.To(b.Shoot.GetReplicas(1)),
			Prometheus:        valuesPrometheus,
			KubeStateMetrics:  valuesKubeStateMetrics,
		}
		valuesUpdater = vpa.ValuesUpdater{
			Image:             imageUpdater.String(),
			PriorityClassName: v1beta1constants.PriorityClassNameShootControlPlane200,
			Replicas:          ptr.To(b.Shoot.GetReplicas(1)),
		}
	)

	if vpaConfig := b.Shoot.GetInfo().Spec.Kubernetes.VerticalPodAutoscaler; vpaConfig != nil {
		valuesRecommender.Interval = vpaConfig.RecommenderInterval
		valuesRecommender.RecommendationMarginFraction = vpaConfig.RecommendationMarginFraction
		valuesRecommender.TargetCPUPercentile = vpaConfig.TargetCPUPercentile
		valuesRecommender.RecommendationLowerBoundCPUPercentile = vpaConfig.RecommendationLowerBoundCPUPercentile
		valuesRecommender.RecommendationUpperBoundCPUPercentile = vpaConfig.RecommendationUpperBoundCPUPercentile
		valuesRecommender.CPUHistogramDecayHalfLife = vpaConfig.CPUHistogramDecayHalfLife
		valuesRecommender.TargetMemoryPercentile = vpaConfig.TargetMemoryPercentile
		valuesRecommender.RecommendationLowerBoundMemoryPercentile = vpaConfig.RecommendationLowerBoundMemoryPercentile
		valuesRecommender.RecommendationUpperBoundMemoryPercentile = vpaConfig.RecommendationUpperBoundMemoryPercentile
		valuesRecommender.MemoryHistogramDecayHalfLife = vpaConfig.MemoryHistogramDecayHalfLife
		valuesRecommender.MemoryAggregationInterval = vpaConfig.MemoryAggregationInterval
		valuesRecommender.MemoryAggregationIntervalCount = vpaConfig.MemoryAggregationIntervalCount

		valuesUpdater.EvictAfterOOMThreshold = vpaConfig.EvictAfterOOMThreshold
		valuesUpdater.EvictionRateBurst = vpaConfig.EvictionRateBurst
		valuesUpdater.EvictionRateLimit = vpaConfig.EvictionRateLimit
		valuesUpdater.EvictionTolerance = vpaConfig.EvictionTolerance
		valuesUpdater.Interval = vpaConfig.UpdaterInterval
	}

	return vpa.New(
		b.SeedClientSet.Client(),
		b.Shoot.SeedNamespace,
		b.SecretsManager,
		vpa.Values{
			ClusterType:              component.ClusterTypeShoot,
			Enabled:                  true,
			SecretNameServerCA:       v1beta1constants.SecretNameCACluster,
			RuntimeKubernetesVersion: b.Seed.KubernetesVersion,
			AdmissionController:      valuesAdmissionController,
			Recommender:              valuesRecommender,
			Updater:                  valuesUpdater,
		},
	), nil
}

// DeployVerticalPodAutoscaler deploys or destroys the VPA to the shoot namespace in the seed.
func (b *Botanist) DeployVerticalPodAutoscaler(ctx context.Context) error {
	if !b.Shoot.WantsVerticalPodAutoscaler {
		return b.Shoot.Components.ControlPlane.VerticalPodAutoscaler.Destroy(ctx)
	}

	return b.Shoot.Components.ControlPlane.VerticalPodAutoscaler.Deploy(ctx)
}
