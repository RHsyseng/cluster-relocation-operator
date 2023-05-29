/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterRelocationSpec defines the desired state of ClusterRelocation
type ClusterRelocationSpec struct {
	// Important: Run "make" to regenerate code after modifying this file

	// ApiCertRef is a reference to a TLS secret that will be used for the API server.
	// If it is omitted, a self-signed certificate will be generated.
	ApiCertRef *KubeRef `json:"apiCertRef,omitempty"`

	// CatalogSources define new CatalogSources to install on the cluster.
	// If defined, existing CatalogSources will be removed.
	CatalogSources []CatalogSources `json:"catalogSources,omitempty"`

	// Domain defines the new base domain for the cluster.
	Domain string `json:"domain"`

	// ImageDigestSources will be converted into ImageContentSourcePolicys on the cluster.
	// If defined, existing ImageContentSourcePolicys will be removed.
	ImageDigestSources []ImageDigestSources `json:"imageDigestSources,omitempty"`

	// IngressCertRef is a reference to a TLS secret that will be used for the Ingress Controller.
	// If it is omitted, a self-signed certificate will be generated.
	IngressCertRef *KubeRef `json:"ingressCertRef,omitempty"`

	// PullSecretRef is a reference to new cluster-wide pull secret.
	// If defined, it will replace the secret located at openshift-config/pull-secret.
	PullSecretRef *KubeRef `json:"pullSecretRef,omitempty"`

	// RegistryCertRef is a reference to a ConfigMap with a new trusted certificate.
	// It will be added to image.config.openshift.io/cluster (additionalTrustedCA).
	RegistryCertRef *KubeRef `json:"registryCertRef,omitempty"`

	// SSHKeys defines a list of authorized SSH keys for the 'core' user.
	// If defined, it will replace the existing authorized SSH key(s).
	SSHKeys []string `json:"sshKeys,omitempty"`
}

// ClusterRelocationStatus defines the observed state of ClusterRelocation
type ClusterRelocationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster

// ClusterRelocation is the Schema for the clusterrelocations API
type ClusterRelocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterRelocationSpec   `json:"spec"`
	Status ClusterRelocationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClusterRelocationList contains a list of ClusterRelocation
type ClusterRelocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterRelocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterRelocation{}, &ClusterRelocationList{})
}

type KubeRef struct {
	// Namespace is the namespace of the referenced object.
	Namespace string `json:"namespace"`

	// Name is the name of the referenced object.
	Name string `json:"name"`
}

type ImageDigestSources struct {
	// Mirrors is one or more repositories that may also contain the same images.
	Mirrors []string `json:"mirrors"`

	// Source is the repository that users refer to, e.g. in image pull specifications.
	Source string `json:"source"`
}

type CatalogSources struct {
	// Name is the name of the CatalogSource.
	Name string `json:"name"`

	// Image is an operator-registry container image to instantiate a registry-server with.
	Image string `json:"image"`
}
