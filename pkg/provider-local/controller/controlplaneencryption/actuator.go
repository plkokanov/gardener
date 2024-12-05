// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	apiserverconfigv1 "k8s.io/apiserver/pkg/apis/apiserver/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/controlplaneencryption"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/utils"
)

type actuator struct {
	client client.Client
}

// NewActuator creates a new Actuator that updates the status of the handled DNSRecord resources.
func NewActuator(mgr manager.Manager) controlplaneencryption.Actuator {
	return &actuator{
		client: mgr.GetClient(),
	}
}

func (a *actuator) Reconcile(ctx context.Context, _ logr.Logger, encryption *extensionsv1alpha1.ControlPlaneEncryption, _ *extensionscontroller.Cluster) error {
	if encryption.Spec.ProviderConfig == nil {
		return errors.New("empty providerconfig")
	}

	providerName := utils.ComputeSHA256Hex(encryption.Spec.ProviderConfig.Raw)
	if len(providerName) > 20 {
		providerName = providerName[:20]
	}

	patch := client.MergeFrom(encryption.DeepCopy())
	encryption.Status = extensionsv1alpha1.ControlPlaneEncryptionStatus{
		ActiveProviderEncryptionConfigs: []*runtime.RawExtension{encryption.Spec.ProviderConfig},
		APIServerKMSEncryptionConfigs: []apiserverconfigv1.KMSConfiguration{
			{
				APIVersion: "v2",
				Name:       fmt.Sprintf("kms-local-%s", providerName),
				Endpoint: (&url.URL{
					Scheme: "unix",
					Path:   fmt.Sprintf("/var/run/kmsplugin-%s/socket.sock", providerName),
				}).String(),
			},
		},
	}
	return a.client.Status().Patch(ctx, encryption, patch)
}

func (a *actuator) Delete(_ context.Context, _ logr.Logger, _ *extensionsv1alpha1.ControlPlaneEncryption, _ *extensionscontroller.Cluster) error {
	return nil
}

func (a *actuator) ForceDelete(ctx context.Context, logger logr.Logger, encryption *extensionsv1alpha1.ControlPlaneEncryption, cluster *extensionscontroller.Cluster) error {
	return a.Delete(ctx, logger, encryption, cluster)
}
