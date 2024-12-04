// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package local

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlPlaneEncryptionConfig contains provider-specific controlplane encryption config
type ControlPlaneEncryptionConfig struct {
	metav1.TypeMeta
	// Key is the key for the controlplane encryption
	Key string
}
