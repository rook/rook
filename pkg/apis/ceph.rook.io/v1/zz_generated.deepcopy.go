// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

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

// Code generated by deepcopy-gen. DO NOT EDIT.

package v1

import (
	v1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephBlockPool) DeepCopyInto(out *CephBlockPool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Status != nil {
		in, out := &in.Status, &out.Status
		*out = new(Status)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephBlockPool.
func (in *CephBlockPool) DeepCopy() *CephBlockPool {
	if in == nil {
		return nil
	}
	out := new(CephBlockPool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephBlockPool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephBlockPoolList) DeepCopyInto(out *CephBlockPoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephBlockPool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephBlockPoolList.
func (in *CephBlockPoolList) DeepCopy() *CephBlockPoolList {
	if in == nil {
		return nil
	}
	out := new(CephBlockPoolList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephBlockPoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephClient) DeepCopyInto(out *CephClient) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephClient.
func (in *CephClient) DeepCopy() *CephClient {
	if in == nil {
		return nil
	}
	out := new(CephClient)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephClient) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephClientList) DeepCopyInto(out *CephClientList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephClient, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephClientList.
func (in *CephClientList) DeepCopy() *CephClientList {
	if in == nil {
		return nil
	}
	out := new(CephClientList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephClientList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephCluster) DeepCopyInto(out *CephCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephCluster.
func (in *CephCluster) DeepCopy() *CephCluster {
	if in == nil {
		return nil
	}
	out := new(CephCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephClusterList) DeepCopyInto(out *CephClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephClusterList.
func (in *CephClusterList) DeepCopy() *CephClusterList {
	if in == nil {
		return nil
	}
	out := new(CephClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephFilesystem) DeepCopyInto(out *CephFilesystem) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Status != nil {
		in, out := &in.Status, &out.Status
		*out = new(Status)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephFilesystem.
func (in *CephFilesystem) DeepCopy() *CephFilesystem {
	if in == nil {
		return nil
	}
	out := new(CephFilesystem)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephFilesystem) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephFilesystemList) DeepCopyInto(out *CephFilesystemList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephFilesystem, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephFilesystemList.
func (in *CephFilesystemList) DeepCopy() *CephFilesystemList {
	if in == nil {
		return nil
	}
	out := new(CephFilesystemList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephFilesystemList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephHealthMessage) DeepCopyInto(out *CephHealthMessage) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephHealthMessage.
func (in *CephHealthMessage) DeepCopy() *CephHealthMessage {
	if in == nil {
		return nil
	}
	out := new(CephHealthMessage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephNFS) DeepCopyInto(out *CephNFS) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Status != nil {
		in, out := &in.Status, &out.Status
		*out = new(Status)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephNFS.
func (in *CephNFS) DeepCopy() *CephNFS {
	if in == nil {
		return nil
	}
	out := new(CephNFS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephNFS) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephNFSList) DeepCopyInto(out *CephNFSList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephNFS, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephNFSList.
func (in *CephNFSList) DeepCopy() *CephNFSList {
	if in == nil {
		return nil
	}
	out := new(CephNFSList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephNFSList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephObjectStore) DeepCopyInto(out *CephObjectStore) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Status != nil {
		in, out := &in.Status, &out.Status
		*out = new(Status)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephObjectStore.
func (in *CephObjectStore) DeepCopy() *CephObjectStore {
	if in == nil {
		return nil
	}
	out := new(CephObjectStore)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephObjectStore) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephObjectStoreList) DeepCopyInto(out *CephObjectStoreList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephObjectStore, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephObjectStoreList.
func (in *CephObjectStoreList) DeepCopy() *CephObjectStoreList {
	if in == nil {
		return nil
	}
	out := new(CephObjectStoreList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephObjectStoreList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephObjectStoreUser) DeepCopyInto(out *CephObjectStoreUser) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Status != nil {
		in, out := &in.Status, &out.Status
		*out = new(Status)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephObjectStoreUser.
func (in *CephObjectStoreUser) DeepCopy() *CephObjectStoreUser {
	if in == nil {
		return nil
	}
	out := new(CephObjectStoreUser)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephObjectStoreUser) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephObjectStoreUserList) DeepCopyInto(out *CephObjectStoreUserList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]CephObjectStoreUser, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephObjectStoreUserList.
func (in *CephObjectStoreUserList) DeepCopy() *CephObjectStoreUserList {
	if in == nil {
		return nil
	}
	out := new(CephObjectStoreUserList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *CephObjectStoreUserList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephStatus) DeepCopyInto(out *CephStatus) {
	*out = *in
	if in.Details != nil {
		in, out := &in.Details, &out.Details
		*out = make(map[string]CephHealthMessage, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephStatus.
func (in *CephStatus) DeepCopy() *CephStatus {
	if in == nil {
		return nil
	}
	out := new(CephStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CephVersionSpec) DeepCopyInto(out *CephVersionSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CephVersionSpec.
func (in *CephVersionSpec) DeepCopy() *CephVersionSpec {
	if in == nil {
		return nil
	}
	out := new(CephVersionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClientSpec) DeepCopyInto(out *ClientSpec) {
	*out = *in
	if in.Caps != nil {
		in, out := &in.Caps, &out.Caps
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClientSpec.
func (in *ClientSpec) DeepCopy() *ClientSpec {
	if in == nil {
		return nil
	}
	out := new(ClientSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterSpec) DeepCopyInto(out *ClusterSpec) {
	*out = *in
	out.CephVersion = in.CephVersion
	in.Storage.DeepCopyInto(&out.Storage)
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(v1alpha2.AnnotationsSpec, len(*in))
		for key, val := range *in {
			var outVal map[string]string
			if val == nil {
				(*out)[key] = nil
			} else {
				in, out := &val, &outVal
				*out = make(v1alpha2.Annotations, len(*in))
				for key, val := range *in {
					(*out)[key] = val
				}
			}
			(*out)[key] = outVal
		}
	}
	if in.Placement != nil {
		in, out := &in.Placement, &out.Placement
		*out = make(v1alpha2.PlacementSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	in.Network.DeepCopyInto(&out.Network)
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make(v1alpha2.ResourceSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = *val.DeepCopy()
		}
	}
	if in.PriorityClassNames != nil {
		in, out := &in.PriorityClassNames, &out.PriorityClassNames
		*out = make(v1alpha2.PriorityClassNamesSpec, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	out.DisruptionManagement = in.DisruptionManagement
	in.Mon.DeepCopyInto(&out.Mon)
	out.RBDMirroring = in.RBDMirroring
	out.CrashCollector = in.CrashCollector
	out.Dashboard = in.Dashboard
	out.Monitoring = in.Monitoring
	out.External = in.External
	in.Mgr.DeepCopyInto(&out.Mgr)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterSpec.
func (in *ClusterSpec) DeepCopy() *ClusterSpec {
	if in == nil {
		return nil
	}
	out := new(ClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterStatus) DeepCopyInto(out *ClusterStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.CephStatus != nil {
		in, out := &in.CephStatus, &out.CephStatus
		*out = new(CephStatus)
		(*in).DeepCopyInto(*out)
	}
	if in.CephVersion != nil {
		in, out := &in.CephVersion, &out.CephVersion
		*out = new(ClusterVersion)
		**out = **in
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterStatus.
func (in *ClusterStatus) DeepCopy() *ClusterStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ClusterVersion) DeepCopyInto(out *ClusterVersion) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ClusterVersion.
func (in *ClusterVersion) DeepCopy() *ClusterVersion {
	if in == nil {
		return nil
	}
	out := new(ClusterVersion)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Condition) DeepCopyInto(out *Condition) {
	*out = *in
	in.LastHeartbeatTime.DeepCopyInto(&out.LastHeartbeatTime)
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Condition.
func (in *Condition) DeepCopy() *Condition {
	if in == nil {
		return nil
	}
	out := new(Condition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CrashCollectorSpec) DeepCopyInto(out *CrashCollectorSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CrashCollectorSpec.
func (in *CrashCollectorSpec) DeepCopy() *CrashCollectorSpec {
	if in == nil {
		return nil
	}
	out := new(CrashCollectorSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DashboardSpec) DeepCopyInto(out *DashboardSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DashboardSpec.
func (in *DashboardSpec) DeepCopy() *DashboardSpec {
	if in == nil {
		return nil
	}
	out := new(DashboardSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DisruptionManagementSpec) DeepCopyInto(out *DisruptionManagementSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DisruptionManagementSpec.
func (in *DisruptionManagementSpec) DeepCopy() *DisruptionManagementSpec {
	if in == nil {
		return nil
	}
	out := new(DisruptionManagementSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ErasureCodedSpec) DeepCopyInto(out *ErasureCodedSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ErasureCodedSpec.
func (in *ErasureCodedSpec) DeepCopy() *ErasureCodedSpec {
	if in == nil {
		return nil
	}
	out := new(ErasureCodedSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ExternalSpec) DeepCopyInto(out *ExternalSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ExternalSpec.
func (in *ExternalSpec) DeepCopy() *ExternalSpec {
	if in == nil {
		return nil
	}
	out := new(ExternalSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *FilesystemSpec) DeepCopyInto(out *FilesystemSpec) {
	*out = *in
	out.MetadataPool = in.MetadataPool
	if in.DataPools != nil {
		in, out := &in.DataPools, &out.DataPools
		*out = make([]PoolSpec, len(*in))
		copy(*out, *in)
	}
	in.MetadataServer.DeepCopyInto(&out.MetadataServer)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new FilesystemSpec.
func (in *FilesystemSpec) DeepCopy() *FilesystemSpec {
	if in == nil {
		return nil
	}
	out := new(FilesystemSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GaneshaRADOSSpec) DeepCopyInto(out *GaneshaRADOSSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GaneshaRADOSSpec.
func (in *GaneshaRADOSSpec) DeepCopy() *GaneshaRADOSSpec {
	if in == nil {
		return nil
	}
	out := new(GaneshaRADOSSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GaneshaServerSpec) DeepCopyInto(out *GaneshaServerSpec) {
	*out = *in
	in.Placement.DeepCopyInto(&out.Placement)
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(v1alpha2.Annotations, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GaneshaServerSpec.
func (in *GaneshaServerSpec) DeepCopy() *GaneshaServerSpec {
	if in == nil {
		return nil
	}
	out := new(GaneshaServerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewaySpec) DeepCopyInto(out *GatewaySpec) {
	*out = *in
	in.Placement.DeepCopyInto(&out.Placement)
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(v1alpha2.Annotations, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewaySpec.
func (in *GatewaySpec) DeepCopy() *GatewaySpec {
	if in == nil {
		return nil
	}
	out := new(GatewaySpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MetadataServerSpec) DeepCopyInto(out *MetadataServerSpec) {
	*out = *in
	in.Placement.DeepCopyInto(&out.Placement)
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(v1alpha2.Annotations, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Resources.DeepCopyInto(&out.Resources)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MetadataServerSpec.
func (in *MetadataServerSpec) DeepCopy() *MetadataServerSpec {
	if in == nil {
		return nil
	}
	out := new(MetadataServerSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MgrSpec) DeepCopyInto(out *MgrSpec) {
	*out = *in
	if in.Modules != nil {
		in, out := &in.Modules, &out.Modules
		*out = make([]Module, len(*in))
		copy(*out, *in)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MgrSpec.
func (in *MgrSpec) DeepCopy() *MgrSpec {
	if in == nil {
		return nil
	}
	out := new(MgrSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Module) DeepCopyInto(out *Module) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Module.
func (in *Module) DeepCopy() *Module {
	if in == nil {
		return nil
	}
	out := new(Module)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonSpec) DeepCopyInto(out *MonSpec) {
	*out = *in
	if in.VolumeClaimTemplate != nil {
		in, out := &in.VolumeClaimTemplate, &out.VolumeClaimTemplate
		*out = new(corev1.PersistentVolumeClaim)
		(*in).DeepCopyInto(*out)
	}
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonSpec.
func (in *MonSpec) DeepCopy() *MonSpec {
	if in == nil {
		return nil
	}
	out := new(MonSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *MonitoringSpec) DeepCopyInto(out *MonitoringSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new MonitoringSpec.
func (in *MonitoringSpec) DeepCopy() *MonitoringSpec {
	if in == nil {
		return nil
	}
	out := new(MonitoringSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NFSGaneshaSpec) DeepCopyInto(out *NFSGaneshaSpec) {
	*out = *in
	out.RADOS = in.RADOS
	in.Server.DeepCopyInto(&out.Server)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NFSGaneshaSpec.
func (in *NFSGaneshaSpec) DeepCopy() *NFSGaneshaSpec {
	if in == nil {
		return nil
	}
	out := new(NFSGaneshaSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *NetworkSpec) DeepCopyInto(out *NetworkSpec) {
	*out = *in
	in.NetworkSpec.DeepCopyInto(&out.NetworkSpec)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new NetworkSpec.
func (in *NetworkSpec) DeepCopy() *NetworkSpec {
	if in == nil {
		return nil
	}
	out := new(NetworkSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ObjectStoreSpec) DeepCopyInto(out *ObjectStoreSpec) {
	*out = *in
	out.MetadataPool = in.MetadataPool
	out.DataPool = in.DataPool
	in.Gateway.DeepCopyInto(&out.Gateway)
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ObjectStoreSpec.
func (in *ObjectStoreSpec) DeepCopy() *ObjectStoreSpec {
	if in == nil {
		return nil
	}
	out := new(ObjectStoreSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ObjectStoreUserSpec) DeepCopyInto(out *ObjectStoreUserSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ObjectStoreUserSpec.
func (in *ObjectStoreUserSpec) DeepCopy() *ObjectStoreUserSpec {
	if in == nil {
		return nil
	}
	out := new(ObjectStoreUserSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PoolSpec) DeepCopyInto(out *PoolSpec) {
	*out = *in
	out.Replicated = in.Replicated
	out.ErasureCoded = in.ErasureCoded
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PoolSpec.
func (in *PoolSpec) DeepCopy() *PoolSpec {
	if in == nil {
		return nil
	}
	out := new(PoolSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *RBDMirroringSpec) DeepCopyInto(out *RBDMirroringSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new RBDMirroringSpec.
func (in *RBDMirroringSpec) DeepCopy() *RBDMirroringSpec {
	if in == nil {
		return nil
	}
	out := new(RBDMirroringSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ReplicatedSpec) DeepCopyInto(out *ReplicatedSpec) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ReplicatedSpec.
func (in *ReplicatedSpec) DeepCopy() *ReplicatedSpec {
	if in == nil {
		return nil
	}
	out := new(ReplicatedSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Status) DeepCopyInto(out *Status) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Status.
func (in *Status) DeepCopy() *Status {
	if in == nil {
		return nil
	}
	out := new(Status)
	in.DeepCopyInto(out)
	return out
}
