// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package botanist

import (
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/gardener/pkg/component/extensions/containerruntime"
	"github.com/gardener/gardener/pkg/component/extensions/controlplaneencryption"
)

// DefaultControlPlaneEncryption creates the default deployer for the ControlPlaneEncryption custom resource.
func (b *Botanist) DefaultControlPlaneEncryption() controlplaneencryption.Interface {
	var (
		providerConfig *runtime.RawExtension
		providerType   string
	)
	if apiServer := b.Shoot.GetInfo().Spec.Kubernetes.KubeAPIServer; apiServer != nil && apiServer.EncryptionConfig != nil {
		providerConfig = apiServer.EncryptionConfig.ProviderConfig
		providerType = apiServer.EncryptionConfig.Type
	}

	return controlplaneencryption.New(
		b.Logger,
		b.SeedClientSet.Client(),
		&controlplaneencryption.Values{
			Name:           b.Shoot.GetInfo().Name,
			Namespace:      b.Shoot.SeedNamespace,
			Type:           providerType,
			ProviderConfig: providerConfig,
		},
		containerruntime.DefaultInterval,
		containerruntime.DefaultSevereThreshold,
		containerruntime.DefaultTimeout,
	)
}
