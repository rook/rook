/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package leakguard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	cephv1client "github.com/rook/rook/pkg/client/clientset/versioned/typed/ceph.rook.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

type listFunc func(ctx context.Context, c cephv1client.CephV1Interface, ns string) (runtime.Object, error)

type cephKind struct {
	kind string
	list listFunc
}

// lister adapts a getter of CephV1Interface, such as
// CephV1Interface.CephClusters, to a listFunc; L is its List's result type.
func lister[L runtime.Object, C interface {
	List(context.Context, metav1.ListOptions) (L, error)
}](get func(cephv1client.CephV1Interface, string) C,
) listFunc {
	return func(ctx context.Context, c cephv1client.CephV1Interface, ns string) (runtime.Object, error) {
		return get(c, ns).List(ctx, metav1.ListOptions{})
	}
}

// cephKinds lists every kind of CephV1Interface, all namespaced, whose
// status Snapshot writes.
var cephKinds = []cephKind{
	{"CephBlockPool", lister[*cephv1.CephBlockPoolList](cephv1client.CephV1Interface.CephBlockPools)},
	{"CephBlockPoolRadosNamespace", lister[*cephv1.CephBlockPoolRadosNamespaceList](cephv1client.CephV1Interface.CephBlockPoolRadosNamespaces)},
	{"CephBucketNotification", lister[*cephv1.CephBucketNotificationList](cephv1client.CephV1Interface.CephBucketNotifications)},
	{"CephBucketTopic", lister[*cephv1.CephBucketTopicList](cephv1client.CephV1Interface.CephBucketTopics)},
	{"CephCOSIDriver", lister[*cephv1.CephCOSIDriverList](cephv1client.CephV1Interface.CephCOSIDrivers)},
	{"CephClient", lister[*cephv1.CephClientList](cephv1client.CephV1Interface.CephClients)},
	{"CephCluster", lister[*cephv1.CephClusterList](cephv1client.CephV1Interface.CephClusters)},
	{"CephFilesystem", lister[*cephv1.CephFilesystemList](cephv1client.CephV1Interface.CephFilesystems)},
	{"CephFilesystemMirror", lister[*cephv1.CephFilesystemMirrorList](cephv1client.CephV1Interface.CephFilesystemMirrors)},
	{"CephFilesystemSubVolumeGroup", lister[*cephv1.CephFilesystemSubVolumeGroupList](cephv1client.CephV1Interface.CephFilesystemSubVolumeGroups)},
	{"CephNFS", lister[*cephv1.CephNFSList](cephv1client.CephV1Interface.CephNFSes)},
	{"CephNVMeOFGateway", lister[*cephv1.CephNVMeOFGatewayList](cephv1client.CephV1Interface.CephNVMeOFGateways)},
	{"CephObjectRealm", lister[*cephv1.CephObjectRealmList](cephv1client.CephV1Interface.CephObjectRealms)},
	{"CephObjectStore", lister[*cephv1.CephObjectStoreList](cephv1client.CephV1Interface.CephObjectStores)},
	{"CephObjectStoreAccount", lister[*cephv1.CephObjectStoreAccountList](cephv1client.CephV1Interface.CephObjectStoreAccounts)},
	{"CephObjectStoreUser", lister[*cephv1.CephObjectStoreUserList](cephv1client.CephV1Interface.CephObjectStoreUsers)},
	{"CephObjectZone", lister[*cephv1.CephObjectZoneList](cephv1client.CephV1Interface.CephObjectZones)},
	{"CephObjectZoneGroup", lister[*cephv1.CephObjectZoneGroupList](cephv1client.CephV1Interface.CephObjectZoneGroups)},
	{"CephRBDMirror", lister[*cephv1.CephRBDMirrorList](cephv1client.CephV1Interface.CephRBDMirrors)},
}

// Snapshot writes the data of every ConfigMap, the message of every Event and
// the status of every Ceph custom resource in namespaces to
// <dir>/snapshots/<namespace>_<kind>_<name>.txt, one "field: value" line per
// field. Values are written unescaped, so a secret in one reads as it would in
// a log. A failure to list or write one kind does not stop the others; all are
// returned together.
func Snapshot(ctx context.Context, cs kubernetes.Interface, rcs rookclient.Interface, namespaces []string, dir string) error {
	s := snapshotter{dir: filepath.Join(dir, "snapshots")}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("creating the snapshot directory: %w", err)
	}

	var errs []error
	for _, ns := range namespaces {
		errs = append(errs, s.configMaps(ctx, cs, ns), s.events(ctx, cs, ns))
		for _, k := range cephKinds {
			errs = append(errs, s.cephKind(ctx, rcs.CephV1(), ns, k))
		}
	}
	return errors.Join(errs...)
}

type snapshotter struct {
	dir string
}

func (s snapshotter) configMaps(ctx context.Context, cs kubernetes.Interface, ns string) error {
	list, err := cs.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing ConfigMaps in namespace %s: %w", ns, err)
	}
	var errs []error
	for _, cm := range list.Items {
		errs = append(errs, s.write(ns, "ConfigMap", cm.Name, func(w io.Writer) error {
			for _, key := range slices.Sorted(maps.Keys(cm.Data)) {
				if err := writeField(w, "data."+key, cm.Data[key]); err != nil {
					return err
				}
			}
			for _, key := range slices.Sorted(maps.Keys(cm.BinaryData)) {
				if err := writeField(w, "binaryData."+key, string(cm.BinaryData[key])); err != nil {
					return err
				}
			}
			return nil
		}))
	}
	return errors.Join(errs...)
}

func (s snapshotter) events(ctx context.Context, cs kubernetes.Interface, ns string) error {
	list, err := cs.CoreV1().Events(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing Events in namespace %s: %w", ns, err)
	}
	var errs []error
	for _, ev := range list.Items {
		errs = append(errs, s.write(ns, "Event", ev.Name, func(w io.Writer) error {
			return writeField(w, "message", ev.Message)
		}))
	}
	return errors.Join(errs...)
}

func (s snapshotter) cephKind(ctx context.Context, c cephv1client.CephV1Interface, ns string, k cephKind) error {
	list, err := k.list(ctx, c, ns)
	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		// The kind's CRD is not installed.
		return nil
	}
	if err != nil {
		return fmt.Errorf("listing %s in namespace %s: %w", k.kind, ns, err)
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return fmt.Errorf("listing %s in namespace %s: %w", k.kind, ns, err)
	}

	var errs []error
	for _, item := range items {
		obj, err := meta.Accessor(item)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading a %s in namespace %s: %w", k.kind, ns, err))
			continue
		}
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(item)
		if err != nil {
			errs = append(errs, fmt.Errorf("converting %s %s/%s: %w", k.kind, ns, obj.GetName(), err))
			continue
		}
		status, ok := u["status"]
		if !ok || status == nil {
			continue
		}
		errs = append(errs, s.write(ns, k.kind, obj.GetName(), func(w io.Writer) error {
			return writeFlattened(w, "status", status)
		}))
	}
	return errors.Join(errs...)
}

func (s snapshotter) write(ns, kind, name string, body func(io.Writer) error) error {
	rel := fmt.Sprintf("%s_%s_%s.txt", ns, kind, name)
	f, err := os.OpenFile(filepath.Join(s.dir, rel), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("writing the snapshot of %s %s/%s: %w", kind, ns, name, err)
	}
	w := bufio.NewWriter(f)
	err = errors.Join(body(w), w.Flush(), f.Close())
	if err != nil {
		return fmt.Errorf("writing the snapshot of %s %s/%s: %w", kind, ns, name, err)
	}
	return nil
}

// writeFlattened writes each leaf of an unstructured value as a field named
// by its path, e.g. status.conditions[0].message, with map keys sorted.
func writeFlattened(w io.Writer, path string, v any) error {
	switch v := v.(type) {
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(v)) {
			if err := writeFlattened(w, path+"."+key, v[key]); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for i, elem := range v {
			if err := writeFlattened(w, path+"["+strconv.Itoa(i)+"]", elem); err != nil {
				return err
			}
		}
		return nil
	case string:
		return writeField(w, path, v)
	default:
		return writeField(w, path, fmt.Sprint(v))
	}
}

func writeField(w io.Writer, name, value string) error {
	line := name + ": " + value
	if len(value) == 0 || value[len(value)-1] != '\n' {
		line += "\n"
	}
	_, err := io.WriteString(w, line)
	return err
}
