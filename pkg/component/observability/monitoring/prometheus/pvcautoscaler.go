// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package prometheus

import (
	pvcautoscalerv1alpha1 "github.com/gardener/pvc-autoscaler/api/autoscaling/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// gardenAutoscalerName is the value of the pvc-autoscaler --autoscaler-name flag for the instance running in the garden
// cluster. It mirrors pvcautoscaler.GardenAutoscalerName; it is duplicated here to avoid an import cycle (the
// pvc-autoscaler component imports the garden/seed Prometheus subpackages).
const gardenAutoscalerName = "garden"

func (p *prometheus) persistentVolumeClaimAutoscaler() *pvcautoscalerv1alpha1.PersistentVolumeClaimAutoscaler {
	return &pvcautoscalerv1alpha1.PersistentVolumeClaimAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.name(),
			Namespace: p.namespace,
			Labels:    p.getLabels(),
		},
		Spec: pvcautoscalerv1alpha1.PersistentVolumeClaimAutoscalerSpec{
			AutoscalerName: p.autoscalerName(),
			TargetRef: autoscalingv1.CrossVersionObjectReference{
				APIVersion: monitoringv1.SchemeGroupVersion.String(),
				Kind:       monitoringv1.PrometheusesKind,
				Name:       p.values.Name,
			},
			VolumePolicies: []pvcautoscalerv1alpha1.VolumePolicy{
				{
					MaxCapacity: p.values.PVCAutoscaling.MaxCapacity,
					ScaleUp: &pvcautoscalerv1alpha1.ScalingRules{
						UtilizationThresholdPercent: new(70),
						StepPercent:                 new(10),
						MinStepAbsolute:             new(resource.MustParse("1Gi")),
					},
				},
			},
		},
	}
}

// autoscalerName returns the name of the pvc-autoscaler instance that manages the Prometheus PVCA. It must match the
// --autoscaler-name flag of the corresponding pvc-autoscaler deployment (empty for the seed instance, "garden" for the
// garden cluster instance).
func (p *prometheus) autoscalerName() string {
	if p.values.PVCAutoscaling.IsGardenCluster {
		return gardenAutoscalerName
	}
	return ""
}
