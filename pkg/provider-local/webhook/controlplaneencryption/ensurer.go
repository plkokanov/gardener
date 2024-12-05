// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controlplaneencryption

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/extensions/pkg/webhook/controlplaneencryption"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	api "github.com/gardener/gardener/pkg/provider-local/apis/local"
	"github.com/gardener/gardener/pkg/provider-local/imagevector"
)

const kmsPluginVolumeName = "kmsplugin"

// NewEnsurer creates a new controlplaneencryption ensurer.
func NewEnsurer(mgr manager.Manager, logger logr.Logger) controlplaneencryption.Ensurer {
	return &ensurer{
		client:  mgr.GetClient(),
		logger:  logger.WithName("local-controlplaneencryption-ensurer"),
		decoder: serializer.NewCodecFactory(mgr.GetScheme(), serializer.EnableStrict).UniversalDecoder(),
	}
}

type ensurer struct {
	decoder runtime.Decoder
	client  client.Client
	logger  logr.Logger
}

func (e *ensurer) EnsureKubeApiserverDeployment(_ context.Context, controlPlaneEncryption *extensionsv1alpha1.ControlPlaneEncryption, new *appsv1.Deployment) error {
	cpeConfigs, err := e.decodeControlplaneEncryptionConfigs(controlPlaneEncryption)
	if err != nil {
		return err
	}

	image, err := imagevector.ImageVector().FindImage(imagevector.ImageNameKmsPluginProviderLocal)
	if err != nil {
		return err
	}

	for idx, cpeConfig := range cpeConfigs {
		kmsEncryptionConfig := controlPlaneEncryption.Status.APIServerKMSEncryptionConfigs[idx]
		uri, err := url.Parse(kmsEncryptionConfig.Endpoint)
		if err != nil {
			return err
		}
		path := filepath.Dir(uri.Path)

		new.Spec.Template.Spec.Volumes = webhook.EnsureVolumeWithName(new.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: kmsEncryptionConfig.Name,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})

		// TODO check ha case: find kube-api-sever idx
		new.Spec.Template.Spec.Containers[0].VolumeMounts = webhook.EnsureVolumeMountWithName(new.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      kmsEncryptionConfig.Name,
			MountPath: path,
		})

		new.Spec.Template.Spec.InitContainers = webhook.EnsureContainerWithName(new.Spec.Template.Spec.InitContainers,
			corev1.Container{
				Name:            kmsEncryptionConfig.Name,
				Image:           image.String(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				RestartPolicy:   ptr.To(corev1.ContainerRestartPolicyAlways),
				Args: []string{
					"--listen=" + uri.Path,
					"--key=" + cpeConfig.Key,
				},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      kmsEncryptionConfig.Name,
					MountPath: path,
				}},
			},
		)
	}

	return nil
}

func (e *ensurer) decodeControlplaneEncryptionConfigs(cpe *extensionsv1alpha1.ControlPlaneEncryption) ([]*api.ControlPlaneEncryptionConfig, error) {
	var cpEncryptionConfig []*api.ControlPlaneEncryptionConfig

	if cpe.Status.ActiveProviderEncryptionConfigs == nil {
		return cpEncryptionConfig, nil
	}

	for _, activeConf := range cpe.Status.ActiveProviderEncryptionConfigs {
		encryptionConfig := &api.ControlPlaneEncryptionConfig{}
		if _, _, err := e.decoder.Decode(activeConf.Raw, nil, encryptionConfig); err != nil {
			return nil, fmt.Errorf("could not decode ControlPlaneEncryptionConfig '%s': %w", client.ObjectKeyFromObject(cpe), err)
		}

		cpEncryptionConfig = append(cpEncryptionConfig, encryptionConfig)
	}

	return cpEncryptionConfig, nil
}
