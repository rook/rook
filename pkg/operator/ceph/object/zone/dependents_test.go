package zone

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestCephObjectZoneDependentStores(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))
	namespace := "test-ceph-object-store-dependents"
	zoneGroupName := "zonegroup-a"
	metadataPool := cephv1.PoolSpec{}
	dataPool := cephv1.PoolSpec{}
	clusterInfo := client.AdminTestClusterInfo(namespace)

	objectZoneA := &cephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "zone-a",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectZone",
		},
		Spec: cephv1.ObjectZoneSpec{
			ZoneGroup:    zoneGroupName,
			MetadataPool: metadataPool,
			DataPool:     dataPool,
		},
	}

	objectStoreA := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "store-a",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStore",
		},
		Spec: cephv1.ObjectStoreSpec{},
	}
	objectStoreA.Spec.Zone.Name = "zone-a"
	objectStoreA.Spec.Gateway.Port = 80
	objectStoreB := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "store-b",
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStore",
		},
		Spec: cephv1.ObjectStoreSpec{},
	}
	objectStoreB.Spec.Zone.Name = "zone-b"
	objectStoreB.Spec.Gateway.Port = 80

	var c *clusterd.Context
	executor := &exectest.MockExecutor{}
	newClusterdCtx := func(executor exec.Executor) *clusterd.Context {
		return &clusterd.Context{
			RookClientset: rookclient.NewSimpleClientset(),
			Executor:      executor,
		}
	}
	t.Run("no objectstores exists", func(t *testing.T) {
		c = newClusterdCtx(executor)
		deps, err := CephObjectZoneDependentStores(c, clusterInfo, objectZoneA, object.NewContext(c, clusterInfo, objectZoneA.Name))
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})
	t.Run("one objectstores exists", func(t *testing.T) {
		c = newClusterdCtx(executor)
		_, err := c.RookClientset.CephV1().CephObjectStores(clusterInfo.Namespace).Create(context.TODO(), objectStoreA, v1.CreateOptions{})
		assert.NoError(t, err)
		deps, err := CephObjectZoneDependentStores(c, clusterInfo, objectZoneA, object.NewContext(c, clusterInfo, objectZoneA.Name))
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"store-a"}, deps.OfKind("CephObjectStore"))
	})
	t.Run("objectstore exists for different zone", func(t *testing.T) {
		c = newClusterdCtx(executor)
		_, err := c.RookClientset.CephV1().CephObjectStores(clusterInfo.Namespace).Create(context.TODO(), objectStoreB, v1.CreateOptions{})
		assert.NoError(t, err)
		deps, err := CephObjectZoneDependentStores(c, clusterInfo, objectZoneA, object.NewContext(c, clusterInfo, objectZoneA.Name))
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})
	t.Run("multipleobjectstore exists for zone", func(t *testing.T) {
		c = newClusterdCtx(executor)
		_, err := c.RookClientset.CephV1().CephObjectStores(clusterInfo.Namespace).Create(context.TODO(), objectStoreA, v1.CreateOptions{})
		assert.NoError(t, err)
		objectStoreB.Spec.Zone.Name = "zone-a"
		_, err = c.RookClientset.CephV1().CephObjectStores(clusterInfo.Namespace).Create(context.TODO(), objectStoreB, v1.CreateOptions{})
		assert.NoError(t, err)
		deps, err := CephObjectZoneDependentStores(c, clusterInfo, objectZoneA, object.NewContext(c, clusterInfo, objectZoneA.Name))
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"store-a", "store-b"}, deps.OfKind("CephObjectStore"))
	})
}
