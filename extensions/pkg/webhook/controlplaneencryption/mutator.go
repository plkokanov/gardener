// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener/extensions/pkg/webhook"
	extensionscontextwebhook "github.com/gardener/gardener/extensions/pkg/webhook/context"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
)

// Ensurer ensures that the kube-apiserver deployment conforms to the provider requirements.
type Ensurer interface {
	EnsureKubeApiserverDeployment(ctx context.Context, controlPlaneEncryption *extensionsv1alpha1.ControlPlaneEncryption, deployment *appsv1.Deployment) error
}

// NewMutator creates a new controlplaneencryption mutator.
func NewMutator(mgr manager.Manager, logger logr.Logger, ensurer Ensurer) webhook.Mutator {
	return &mutator{
		client:  mgr.GetClient(),
		logger:  logger.WithName("mutator"),
		ensurer: ensurer,
	}
}

type mutator struct {
	client  client.Client
	logger  logr.Logger
	ensurer Ensurer
}

// Mutate validates and if needed mutates the given object.
func (m *mutator) Mutate(ctx context.Context, new, _ client.Object) error {
	if new.GetDeletionTimestamp() != nil {
		return nil
	}

	gctx := extensionscontextwebhook.NewGardenContext(m.client, new)
	cluster, err := gctx.GetCluster(ctx)
	if err != nil {
		return err
	}

	controlPlaneEncryption := &extensionsv1alpha1.ControlPlaneEncryption{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Shoot.Name,
			Namespace: cluster.ObjectMeta.Name,
		},
	}
	if err := m.client.Get(ctx, client.ObjectKeyFromObject(controlPlaneEncryption), controlPlaneEncryption); err != nil {
		if apierrors.IsNotFound(err) {
			m.logger.Info("Skpping mutation: controlplaneencryption resource does not exist", "controlplaneencryption", client.ObjectKeyFromObject(controlPlaneEncryption))
			return nil
		}
		return fmt.Errorf("failed to get extension '%s': %w", client.ObjectKeyFromObject(controlPlaneEncryption), err)
	}

	newDeployment, ok := new.(*appsv1.Deployment)
	if !ok {
		return fmt.Errorf("could not mutate: object is not of type %q", "Secret")
	}
	if newDeployment.Name != v1beta1constants.DeploymentNameKubeAPIServer {
		return nil
	}

	webhook.LogMutation(m.logger, newDeployment.Kind, newDeployment.Namespace, newDeployment.Name)
	return m.ensurer.EnsureKubeApiserverDeployment(ctx, controlPlaneEncryption, newDeployment)
}
