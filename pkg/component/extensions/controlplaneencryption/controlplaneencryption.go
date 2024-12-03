// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiserverconfigv1 "k8s.io/apiserver/pkg/apis/apiserver/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/extensions"
)

// TimeNow returns the current time. Exposed for testing.
var TimeNow = time.Now

// Interface is the interface for the ControlPlaneEncryption.
type Interface interface {
	component.DeployWaiter
	KubeAPIServerKMSEncryptionConfigurations() []apiserverconfigv1.KMSConfiguration
}

// Values are the values fr the ControlPlaneEncryption.
type Values struct {
	// Namespace is the Shoot namespace in the seed.
	Namespace string
	// Name is the name of the ControlPlane resource. Commonly the Shoot's name.
	Name string
	// Type is the type of the ControlPlane provider.
	Type string
	// ProviderConfig is the provider config of the extension.
	ProviderConfig *runtime.RawExtension
}

// New creates a new instance of Interface.
func New(
	log logr.Logger,
	client client.Client,
	values *Values,
	waitInterval time.Duration,
	waitSevereThreshold time.Duration,
	waitTimeout time.Duration,
) Interface {
	return &controlPlaneEncryption{
		log:                 log,
		client:              client,
		values:              values,
		waitInterval:        waitInterval,
		waitSevereThreshold: waitSevereThreshold,
		waitTimeout:         waitTimeout,

		controlPlaneEncryption: &extensionsv1alpha1.ControlPlaneEncryption{
			ObjectMeta: metav1.ObjectMeta{
				Name:      values.Name,
				Namespace: values.Namespace,
			},
		},
	}
}

type controlPlaneEncryption struct {
	log                 logr.Logger
	client              client.Client
	values              *Values
	waitInterval        time.Duration
	waitSevereThreshold time.Duration
	waitTimeout         time.Duration

	controlPlaneEncryption *extensionsv1alpha1.ControlPlaneEncryption
	kmsConfigurations      []apiserverconfigv1.KMSConfiguration
}

func (c *controlPlaneEncryption) Deploy(ctx context.Context) error {
	var providerConfig *runtime.RawExtension
	if cfg := c.values.ProviderConfig; cfg != nil {
		providerConfig = &runtime.RawExtension{
			Raw: cfg.Raw,
		}
	}

	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, c.controlPlaneEncryption, func() error {
		metav1.SetMetaDataAnnotation(&c.controlPlaneEncryption.ObjectMeta, v1beta1constants.GardenerOperation, v1beta1constants.GardenerOperationReconcile)
		metav1.SetMetaDataAnnotation(&c.controlPlaneEncryption.ObjectMeta, v1beta1constants.GardenerTimestamp, TimeNow().UTC().Format(time.RFC3339Nano))
		c.controlPlaneEncryption.Spec = extensionsv1alpha1.ControlPlaneEncryptionSpec{
			DefaultSpec: extensionsv1alpha1.DefaultSpec{
				Type:           c.values.Type,
				ProviderConfig: providerConfig,
			},
		}
		return nil
	})

	return err
}

func (c *controlPlaneEncryption) Destroy(ctx context.Context) error {
	return extensions.DeleteExtensionObject(
		ctx,
		c.client,
		c.controlPlaneEncryption,
	)
}

func (c *controlPlaneEncryption) Wait(ctx context.Context) error {
	return extensions.WaitUntilExtensionObjectReady(
		ctx,
		c.client,
		c.log,
		c.controlPlaneEncryption,
		extensionsv1alpha1.ControlPlaneEncryptionResource,
		c.waitInterval,
		c.waitSevereThreshold,
		c.waitTimeout,
		func() error {
			c.kmsConfigurations = c.controlPlaneEncryption.Status.APIServerKMSEncryptionConfigs
			return nil
		},
	)
}

func (c *controlPlaneEncryption) WaitCleanup(ctx context.Context) error {
	return extensions.WaitUntilExtensionObjectDeleted(
		ctx,
		c.client,
		c.log,
		c.controlPlaneEncryption,
		extensionsv1alpha1.ControlPlaneEncryptionResource,
		c.waitInterval,
		c.waitTimeout,
	)
}

func (c *controlPlaneEncryption) KubeAPIServerKMSEncryptionConfigurations() []apiserverconfigv1.KMSConfiguration {
	return c.kmsConfigurations
}
