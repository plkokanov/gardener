// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KeyType is the key type.
type KeyType string

const (
	// KeyTypeAES32 is the aes:32 key type.
	KeyTypeAES32 KeyType = "aes:32"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlPlaneEncryptionConfig contains provider-specific controlplane encryption config
type ControlPlaneEncryptionConfig struct {
	metav1.TypeMeta
	// KeyType
	// +optional
	KeyType *KeyType
}
