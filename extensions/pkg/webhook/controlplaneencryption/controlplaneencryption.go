// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
)

const (
	// WebhookName is the webhook name.
	WebhookName = "controlplaneencryption"
)

var logger = log.Log.WithName("controlplaneencryption-webhook")

// Args are arguments for adding a controlplaneencryption webhook to a manager.
type Args struct {
	// Provider is the control plane encryption provider for this webhook
	Provider string
	// Types is a list of resource types.
	Types []extensionswebhook.Type
	// Mutator is a mutator to be used by the admission handler.
	Mutator extensionswebhook.Mutator
	// ObjectSelector is the object selector of the underlying webhook
	ObjectSelector *metav1.LabelSelector
}

// New creates a new controlplaneencryption webhook with the given args.
func New(mgr manager.Manager, args Args) (*extensionswebhook.Webhook, error) {
	logger := logger.WithValues("provider", args.Provider)

	// Create handler
	handler, err := extensionswebhook.NewBuilder(mgr, logger).WithMutator(args.Mutator, args.Types...).Build()
	if err != nil {
		return nil, err
	}

	// Create webhook
	var (
		name = WebhookName
		path = WebhookName
	)

	logger.Info("Creating network webhook", "name", name)
	return &extensionswebhook.Webhook{
		Name:              name,
		Provider:          args.Provider,
		Types:             args.Types,
		Target:            extensionswebhook.TargetSeed,
		Path:              path,
		Webhook:           &admission.Webhook{Handler: handler, RecoverPanic: ptr.To(true)},
		NamespaceSelector: buildNamespaceSelector(args.Provider),
		ObjectSelector:    args.ObjectSelector,
	}, nil
}

func buildNamespaceSelector(provider string) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      v1beta1constants.LabelControlPlaneEncryptionProvider,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{provider},
			},
		},
	}
}
