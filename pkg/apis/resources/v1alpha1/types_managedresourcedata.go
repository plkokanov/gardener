// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:resource:shortName="mrd"
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`,description="creation timestamp"
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ManagedResourceData holds non-sensitive rendered Kubernetes manifests referenced by a ManagedResource via spec.dataRefs.
type ManagedResourceData struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Data contains the rendered manifests as compressed byte arrays.
	// Keys follow the same convention as Secret data keys (e.g., "data.yaml.br" for Brotli-compressed YAML).
	// +optional
	Data map[string][]byte `json:"data,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ManagedResourceDataList is a list of ManagedResourceData resources.
type ManagedResourceDataList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items is the list of ManagedResourceData.
	Items []ManagedResourceData `json:"items"`
}
