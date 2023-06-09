//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	configv1 "github.com/openshift/api/config/v1"
	agentv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ACMRegistration) DeepCopyInto(out *ACMRegistration) {
	*out = *in
	if in.ManagedClusterSet != nil {
		in, out := &in.ManagedClusterSet, &out.ManagedClusterSet
		*out = new(string)
		**out = **in
	}
	out.ACMSecret = in.ACMSecret
	if in.KlusterletAddonConfig != nil {
		in, out := &in.KlusterletAddonConfig, &out.KlusterletAddonConfig
		*out = new(agentv1.KlusterletAddonConfigSpec)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ACMRegistration.
func (in *ACMRegistration) DeepCopy() *ACMRegistration {
	if in == nil {
		return nil
	}
	out := new(ACMRegistration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CatalogSource) DeepCopyInto(out *CatalogSource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CatalogSource.
func (in *CatalogSource) DeepCopy() *CatalogSource {
	if in == nil {
		return nil
	}
	out := new(CatalogSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRelocation) DeepCopyInto(out *ClusterRelocation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRelocation.
func (in *ClusterRelocation) DeepCopy() *ClusterRelocation {
	if in == nil {
		return nil
	}
	out := new(ClusterRelocation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterRelocation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRelocationList) DeepCopyInto(out *ClusterRelocationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ClusterRelocation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRelocationList.
func (in *ClusterRelocationList) DeepCopy() *ClusterRelocationList {
	if in == nil {
		return nil
	}
	out := new(ClusterRelocationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ClusterRelocationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRelocationSpec) DeepCopyInto(out *ClusterRelocationSpec) {
	*out = *in
	if in.ACMRegistration != nil {
		in, out := &in.ACMRegistration, &out.ACMRegistration
		*out = new(ACMRegistration)
		(*in).DeepCopyInto(*out)
	}
	if in.AddInternalDNSEntries != nil {
		in, out := &in.AddInternalDNSEntries, &out.AddInternalDNSEntries
		*out = new(bool)
		**out = **in
	}
	if in.APICertRef != nil {
		in, out := &in.APICertRef, &out.APICertRef
		*out = new(v1.SecretReference)
		**out = **in
	}
	if in.CatalogSources != nil {
		in, out := &in.CatalogSources, &out.CatalogSources
		*out = make([]CatalogSource, len(*in))
		copy(*out, *in)
	}
	if in.ImageDigestMirrors != nil {
		in, out := &in.ImageDigestMirrors, &out.ImageDigestMirrors
		*out = make([]configv1.ImageDigestMirrors, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.IngressCertRef != nil {
		in, out := &in.IngressCertRef, &out.IngressCertRef
		*out = new(v1.SecretReference)
		**out = **in
	}
	if in.PullSecretRef != nil {
		in, out := &in.PullSecretRef, &out.PullSecretRef
		*out = new(v1.SecretReference)
		**out = **in
	}
	if in.RegistryCert != nil {
		in, out := &in.RegistryCert, &out.RegistryCert
		*out = new(RegistryCert)
		(*in).DeepCopyInto(*out)
	}
	if in.SSHKeys != nil {
		in, out := &in.SSHKeys, &out.SSHKeys
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRelocationSpec.
func (in *ClusterRelocationSpec) DeepCopy() *ClusterRelocationSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterRelocationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterRelocationStatus) DeepCopyInto(out *ClusterRelocationStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterRelocationStatus.
func (in *ClusterRelocationStatus) DeepCopy() *ClusterRelocationStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterRelocationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RegistryCert) DeepCopyInto(out *RegistryCert) {
	*out = *in
	if in.RegistryPort != nil {
		in, out := &in.RegistryPort, &out.RegistryPort
		*out = new(int)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RegistryCert.
func (in *RegistryCert) DeepCopy() *RegistryCert {
	if in == nil {
		return nil
	}
	out := new(RegistryCert)
	in.DeepCopyInto(out)
	return out
}
