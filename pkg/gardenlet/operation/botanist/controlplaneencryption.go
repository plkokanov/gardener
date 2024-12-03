// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package botanist

import (
	"github.com/gardener/gardener/pkg/component/extensions/containerruntime"
	"github.com/gardener/gardener/pkg/component/extensions/controlplaneencryption"
)

// DefaultControlPlaneEncryption creates the default deployer for the ControlPlaneEncryption custom resource.
func (b *Botanist) DefaultControlPlaneEncryption() controlplaneencryption.Interface {
	return controlplaneencryption.New(
		b.Logger,
		b.SeedClientSet.Client(),
		&controlplaneencryption.Values{
			Name:           b.Shoot.GetInfo().Name,
			Namespace:      b.Shoot.SeedNamespace,
			ProviderConfig: b.Shoot.GetInfo().Spec.Kubernetes.KubeAPIServer.EncryptionConfig.ProviderConfig,
			Type:           b.Shoot.GetInfo().Spec.Kubernetes.KubeAPIServer.EncryptionConfig.Type,
		},
		containerruntime.DefaultInterval,
		containerruntime.DefaultSevereThreshold,
		containerruntime.DefaultTimeout,
	)
}
