// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/extensions/pkg/webhook/controlplaneencryption"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/provider-local/local"
)

// WebhookName is the name of the ControlPlaneEncryption webhook.
const WebhookName = "controlplaneencryption"

var (
	logger = log.Log.WithName("local-controlplaneencryption-webhook")

	// DefaultAddOptions are the default AddOptions for AddToManager.
	DefaultAddOptions = AddOptions{}
)

// AddOptions are options to apply when adding the local exposure webhook to the manager.
type AddOptions struct{}

// AddToManagerWithOptions creates a webhook with the given options and adds it to the manager.
func AddToManagerWithOptions(mgr manager.Manager, _ AddOptions) (*extensionswebhook.Webhook, error) {
	logger.Info("Adding webhook to manager")
	return controlplaneencryption.New(mgr, controlplaneencryption.Args{
		Provider: local.Type,
		Types: []extensionswebhook.Type{
			{Obj: &appsv1.Deployment{}},
		},
		ObjectSelector: &metav1.LabelSelector{MatchLabels: map[string]string{v1beta1constants.LabelExtensionProviderMutatedByControlPlaneEncryptionWebhook: "true"}},
		Mutator:        controlplaneencryption.NewMutator(mgr, logger, NewEnsurer(mgr, logger)),
	})
}

// AddToManager creates a webhook with the default options and adds it to the manager.
func AddToManager(mgr manager.Manager) (*extensionswebhook.Webhook, error) {
	return AddToManagerWithOptions(mgr, DefaultAddOptions)
}
