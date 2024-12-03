package botanist

import (
	"github.com/gardener/gardener/pkg/component/extensions/containerruntime"
	"github.com/gardener/gardener/pkg/component/extensions/controlplaneencryption"
)

// DefaultContainerRuntime creates the default deployer for the ContainerRuntime custom resource.
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
