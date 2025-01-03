/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	"context"

	v1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	scheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// CephObjectZoneGroupsGetter has a method to return a CephObjectZoneGroupInterface.
// A group's client should implement this interface.
type CephObjectZoneGroupsGetter interface {
	CephObjectZoneGroups(namespace string) CephObjectZoneGroupInterface
}

// CephObjectZoneGroupInterface has methods to work with CephObjectZoneGroup resources.
type CephObjectZoneGroupInterface interface {
	Create(ctx context.Context, cephObjectZoneGroup *v1.CephObjectZoneGroup, opts metav1.CreateOptions) (*v1.CephObjectZoneGroup, error)
	Update(ctx context.Context, cephObjectZoneGroup *v1.CephObjectZoneGroup, opts metav1.UpdateOptions) (*v1.CephObjectZoneGroup, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.CephObjectZoneGroup, error)
	List(ctx context.Context, opts metav1.ListOptions) (*v1.CephObjectZoneGroupList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.CephObjectZoneGroup, err error)
	CephObjectZoneGroupExpansion
}

// cephObjectZoneGroups implements CephObjectZoneGroupInterface
type cephObjectZoneGroups struct {
	*gentype.ClientWithList[*v1.CephObjectZoneGroup, *v1.CephObjectZoneGroupList]
}

// newCephObjectZoneGroups returns a CephObjectZoneGroups
func newCephObjectZoneGroups(c *CephV1Client, namespace string) *cephObjectZoneGroups {
	return &cephObjectZoneGroups{
		gentype.NewClientWithList[*v1.CephObjectZoneGroup, *v1.CephObjectZoneGroupList](
			"cephobjectzonegroups",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *v1.CephObjectZoneGroup { return &v1.CephObjectZoneGroup{} },
			func() *v1.CephObjectZoneGroupList { return &v1.CephObjectZoneGroupList{} }),
	}
}
