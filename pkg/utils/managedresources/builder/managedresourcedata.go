// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/utils"
)

// ManagedResourceDataBuilder is a structure for managing a ManagedResourceData object.
type ManagedResourceDataBuilder struct {
	client   client.Client
	resource *resourcesv1alpha1.ManagedResourceData
}

// NewManagedResourceData creates a new builder for a ManagedResourceData.
func NewManagedResourceData(client client.Client) *ManagedResourceDataBuilder {
	return &ManagedResourceDataBuilder{
		client:   client,
		resource: &resourcesv1alpha1.ManagedResourceData{},
	}
}

// WithNamespacedName sets the namespace and name.
func (b *ManagedResourceDataBuilder) WithNamespacedName(namespace, name string) *ManagedResourceDataBuilder {
	b.resource.Namespace = namespace
	b.resource.Name = name
	return b
}

// WithLabels sets the labels.
func (b *ManagedResourceDataBuilder) WithLabels(labels map[string]string) *ManagedResourceDataBuilder {
	b.resource.Labels = utils.MergeStringMaps(b.resource.Labels, labels)
	return b
}

// WithData sets the data map (same format as Secret.Data — keys contain serialized manifests).
func (b *ManagedResourceDataBuilder) WithData(data map[string][]byte) *ManagedResourceDataBuilder {
	b.resource.Data = data
	return b
}

func (b *ManagedResourceDataBuilder) BuilderAndName() (string, *ManagedResourceDataBuilder) {
	return b.resource.Name, b
}

// Reconcile creates or updates the ManagedResourceData in-place.
// Unlike Secrets used by ManagedResource, ManagedResourceData objects are mutable and updated
// directly (no unique-name hashing or garbage collection needed).
func (b *ManagedResourceDataBuilder) Reconcile(ctx context.Context) error {
	obj := &resourcesv1alpha1.ManagedResourceData{
		ObjectMeta: metav1.ObjectMeta{Name: b.resource.Name, Namespace: b.resource.Namespace},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, b.client, obj, func() error {
		obj.Labels = b.resource.Labels
		obj.Data = b.resource.Data
		return nil
	})
	return err
}
