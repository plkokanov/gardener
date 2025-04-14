// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package vpaseed

import (
	_ "embed"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	monitoringutils "github.com/gardener/gardener/pkg/component/observability/monitoring/utils"
)

// NetworkPolicyToKubelet returns a NetworkPolicy that allows traffic from the
// cache Prometheus to the kubelet process running on the nodes.
// TODO(plkokanov): Copied from `github.com/gardener/gardener/pkg/component/observability/monitoring/prometheus/cache/networkpolicy.go`
// Find out how to combine the files to avoid duplication
func NetworkPolicyToKubelet(namespace string, nodeCIDR *string) *networkingv1.NetworkPolicy {
	networkPolicy := networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "egress-from-vpa-seed-prometheus-to-kubelet-tcp-10250",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: monitoringutils.Labels(Label),
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To:    []networkingv1.NetworkPolicyPeer{},
				Ports: []networkingv1.NetworkPolicyPort{{Port: ptr.To(intstr.FromInt32(10250)), Protocol: ptr.To(corev1.ProtocolTCP)}},
			}},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}
	if nodeCIDR != nil {
		networkPolicy.Spec.Egress[0].To = []networkingv1.NetworkPolicyPeer{{IPBlock: &networkingv1.IPBlock{CIDR: *nodeCIDR}}}
	}
	return &networkPolicy
}
