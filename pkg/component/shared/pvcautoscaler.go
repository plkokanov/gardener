// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package shared

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener/imagevector"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/component/autoscaling/pvcautoscaler"
	secretsmanager "github.com/gardener/gardener/pkg/utils/secrets/manager"
)

// NewPVCAutoscaler instantiates a new `PVCAutoscaler` component.
func NewPVCAutoscaler(
	c client.Client,
	gardenNamespaceName string,
	secretsManager secretsmanager.Interface,
	enabled bool,
	secretNameServerCA string,
	priorityClassName string,
	isGardenCluster bool,
) (
	deployer component.DeployWaiter,
	err error,
) {
	image, err := imagevector.Containers().FindImage(imagevector.ContainerImageNamePvcAutoscaler)
	if err != nil {
		return nil, err
	}

	deployer = pvcautoscaler.NewPVCAutoscaler(
		c,
		gardenNamespaceName,
		secretsManager,
		pvcautoscaler.Values{
			Image:              image.String(),
			PriorityClassName:  priorityClassName,
			IsGardenCluster:    isGardenCluster,
			SecretNameServerCA: secretNameServerCA,
		},
	)

	if !enabled {
		deployer = component.OpDestroyAndWait(deployer)
	}

	return deployer, nil
}
