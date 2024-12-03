// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	"context"

	"github.com/go-logr/logr"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
)

// Actuator acts upon ControlPlane resources.
type Actuator interface {
	// Reconcile reconciles the ControlPlane.
	Reconcile(context.Context, logr.Logger, *extensionsv1alpha1.ControlPlaneEncryption, *extensionscontroller.Cluster) error
	// Delete deletes the ControlPlaneEncryption.
	Delete(context.Context, logr.Logger, *extensionsv1alpha1.ControlPlaneEncryption, *extensionscontroller.Cluster) error
	// ForceDelete forcefully deletes the ControlPlaneEncryption.
	ForceDelete(context.Context, logr.Logger, *extensionsv1alpha1.ControlPlaneEncryption, *extensionscontroller.Cluster) error
	// TODO: implement
	// Restore restores the ControlPlaneEncryption.
	// Restore(context.Context, logr.Logger, *extensionsv1alpha1.ControlPlaneEncryption, *extensionscontroller.Cluster) (bool, error)
	// Migrate migrates the ControlPlaneEncryption.
	// Migrate(context.Context, logr.Logger, *extensionsv1alpha1.ControlPlaneEncryption, *extensionscontroller.Cluster) error
}
