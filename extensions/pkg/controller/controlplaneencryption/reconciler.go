// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	v1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/controllerutils"
	reconcilerutils "github.com/gardener/gardener/pkg/controllerutils/reconciler"
	"github.com/gardener/gardener/pkg/extensions"
)

type reconciler struct {
	actuator Actuator

	client        client.Client
	reader        client.Reader
	statusUpdater extensionscontroller.StatusUpdater
}

// NewReconciler creates a new reconcile.Reconciler that reconciles
// ControlPlaneEncryption resources of Gardener's `extensions.gardener.cloud` API group.
func NewReconciler(mgr manager.Manager, actuator Actuator) reconcile.Reconciler {
	return reconcilerutils.OperationAnnotationWrapper(
		mgr,
		func() client.Object { return &extensionsv1alpha1.ControlPlaneEncryption{} },
		&reconciler{
			actuator:      actuator,
			client:        mgr.GetClient(),
			reader:        mgr.GetAPIReader(),
			statusUpdater: extensionscontroller.NewStatusUpdater(mgr.GetClient()),
		},
	)
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	enc := &extensionsv1alpha1.ControlPlaneEncryption{}
	if err := r.client.Get(ctx, request.NamespacedName, enc); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Object is gone, stop reconciling")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("error retrieving object from store: %w", err)
	}

	var cluster *extensions.Cluster
	if enc.Namespace != v1beta1constants.GardenNamespace {
		var err error
		cluster, err = extensionscontroller.GetCluster(ctx, r.client, enc.Namespace)
		if err != nil {
			return reconcile.Result{}, err
		}

		if extensionscontroller.IsFailed(cluster) {
			log.Info("Skipping the reconciliation of ControlPlaneEncryption of failed shoot")
			return reconcile.Result{}, nil
		}
	}

	operationType := v1beta1helper.ComputeOperationType(enc.ObjectMeta, enc.Status.LastOperation)

	switch {
	// TODO: implement migrate and restore
	case extensionscontroller.ShouldSkipOperation(operationType, enc):
		return reconcile.Result{}, nil
	case enc.DeletionTimestamp != nil:
		return r.delete(ctx, log, enc, cluster)
	default:
		return r.reconcile(ctx, log, enc, cluster, operationType)
	}
}

func (r *reconciler) reconcile(
	ctx context.Context,
	log logr.Logger,
	enc *extensionsv1alpha1.ControlPlaneEncryption,
	cluster *extensionscontroller.Cluster,
	operationType gardencorev1beta1.LastOperationType,
) (
	reconcile.Result,
	error,
) {
	if !controllerutil.ContainsFinalizer(enc, FinalizerName) {
		log.Info("Adding finalizer")
		if err := controllerutils.AddFinalizers(ctx, r.client, enc, FinalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	if err := r.statusUpdater.Processing(ctx, log, enc, operationType, "Reconciling the ControlPlaneEncryption"); err != nil {
		return reconcile.Result{}, err
	}

	log.Info("Starting the reconciliation of ControlPlaneEncryption")
	if err := r.actuator.Reconcile(ctx, log, enc, cluster); err != nil {
		_ = r.statusUpdater.Error(ctx, log, enc, reconcilerutils.ReconcileErrCauseOrErr(err), operationType, "Error reconciling ControlPlaneEncryption")
		return reconcilerutils.ReconcileErr(err)
	}

	if err := r.statusUpdater.Success(ctx, log, enc, operationType, "Successfully reconciled ControlPlaneEncryption"); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *reconciler) delete(
	ctx context.Context,
	log logr.Logger,
	enc *extensionsv1alpha1.ControlPlaneEncryption,
	cluster *extensionscontroller.Cluster,
) (
	reconcile.Result,
	error,
) {
	if !controllerutil.ContainsFinalizer(enc, FinalizerName) {
		log.Info("Deleting ControlPlaneEncryption causes a no-op as there is no finalizer")
		return reconcile.Result{}, nil
	}

	operationType := v1beta1helper.ComputeOperationType(enc.ObjectMeta, enc.Status.LastOperation)
	if err := r.statusUpdater.Processing(ctx, log, enc, operationType, "Deleting the ControlPlaneEncryption"); err != nil {
		return reconcile.Result{}, err
	}

	log.Info("Starting the deletion of ControlPlaneEncryption")
	var err error
	if cluster != nil && v1beta1helper.ShootNeedsForceDeletion(cluster.Shoot) {
		err = r.actuator.ForceDelete(ctx, log, enc, cluster)
	} else {
		err = r.actuator.Delete(ctx, log, enc, cluster)
	}
	if err != nil {
		_ = r.statusUpdater.Error(ctx, log, enc, reconcilerutils.ReconcileErrCauseOrErr(err), operationType, "Error deleting ControlPlaneEncryption")
		return reconcilerutils.ReconcileErr(err)
	}

	if err := r.statusUpdater.Success(ctx, log, enc, operationType, "Successfully deleted ControlPlaneEncryption"); err != nil {
		return reconcile.Result{}, err
	}

	if controllerutil.ContainsFinalizer(enc, FinalizerName) {
		log.Info("Removing finalizer")
		if err := controllerutils.RemoveFinalizers(ctx, r.client, enc, FinalizerName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return reconcile.Result{}, nil
}
