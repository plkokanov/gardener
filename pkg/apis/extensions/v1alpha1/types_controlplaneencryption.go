// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiserverconfigv1 "k8s.io/apiserver/pkg/apis/apiserver/v1"
)

// ControlPlaneEncryptionResource is a constant for the name of the ControlPlaneEncryption resource.
const ControlPlaneEncryptionResource = "ControlPlaneEncryption"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Namespaced,path=controlplaneencryptions,shortName=cpe,singular=controlplaneencryption
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name=Type,JSONPath=".spec.type",type=string,description="The control plane encryption type."
// +kubebuilder:printcolumn:name=Status,JSONPath=".status.lastOperation.state",type=string,description="Status of control plane encryption resource."
// +kubebuilder:printcolumn:name=Age,JSONPath=".metadata.creationTimestamp",type=date,description="creation timestamp"

// ControlPlaneEncryption is a specification for a ControlPlane resource.
type ControlPlaneEncryption struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the ControlPlane.
	// If the object's deletion timestamp is set, this field is immutable.
	Spec ControlPlaneEncryptionSpec `json:"spec"`
	// +optional
	Status ControlPlaneEncryptionStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlPlaneEncryptionList is a list of ControlPlaneEncryption resources.
type ControlPlaneEncryptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items is the list of ControlPlaneEncryption.
	Items []ControlPlaneEncryption `json:"items"`
}

// GetExtensionSpec implements Object.
func (i *ControlPlaneEncryption) GetExtensionSpec() Spec {
	return &i.Spec
}

// GetExtensionStatus implements Object.
func (i *ControlPlaneEncryption) GetExtensionStatus() Status {
	return &i.Status
}

// ControlPlaneEncryptionSpec is the spec for a ControlPlaneEncryption resource.
type ControlPlaneEncryptionSpec struct {
	// DefaultSpec is a structure containing common fields used by all extension resources.
	DefaultSpec `json:",inline"`
}

// ControlPlaneEncryptionStatus is the status for a ControlPlaneEncryption resource.
type ControlPlaneEncryptionStatus struct {
	// DefaultStatus is a structure containing common fields used by all extension resources.
	DefaultStatus `json:",inline"`
	// ActiveProviderEncryptionConfigs specifies the currently used provider encryption configs.
	// +optional
	ActiveProviderEncryptionConfigs []*runtime.RawExtension `json:"activeProviderEncryptionConfigs,omitempty"`
	// KMSEncryptionConfigs specifies the currently used kms encryption configs.
	// +optional
	APIServerKMSEncryptionConfigs []apiserverconfigv1.KMSConfiguration `json:"apiServerKmsEncryptionConfigs,omitempty"`
}
