package webhook

import (
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	casstest "github.com/rook/rook/pkg/operator/cassandra/test"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"
)

func TestCheckValues(t *testing.T) {

	old := casstest.NewSimpleCluster(3)
	old.Spec.Datacenter.Racks[0].Storage = rookv1alpha2.StorageScopeSpec{
		Selection: rookv1alpha2.Selection{
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{}},
		},
	}

	sameName := old.DeepCopy()
	sameName.Spec.Datacenter.Racks = append(sameName.Spec.Datacenter.Racks, sameName.Spec.Datacenter.Racks[0])

	diskMissing := old.DeepCopy()
	diskMissing.Spec.Datacenter.Racks[0].Storage.VolumeClaimTemplates = nil

	manyDisks := old.DeepCopy()
	manyDisks.Spec.Datacenter.Racks[0].Storage.VolumeClaimTemplates = append(manyDisks.Spec.Datacenter.Racks[0].Storage.VolumeClaimTemplates, manyDisks.Spec.Datacenter.Racks[0].Storage.VolumeClaimTemplates[0])

	configMapSet := old.DeepCopy()
	configMapSet.Spec.Datacenter.Racks[0].ConfigMapName = util.RefFromString("test-configmap")

	tests := []struct {
		name    string
		new     *cassandrav1alpha1.Cluster
		allowed bool
	}{
		{
			name:    "valid",
			new:     old,
			allowed: true,
		},
		{
			name:    "two racks with same name",
			new:     sameName,
			allowed: false,
		},
		{
			name:    "disk missing",
			new:     diskMissing,
			allowed: false,
		},
		{
			name:    "more than one disks in volumeClaimTemplates",
			new:     manyDisks,
			allowed: false,
		},
		{
			name:    "configMapName set",
			new:     configMapSet,
			allowed: false,
		},
	}

	cassadm := &CassandraAdmission{}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, msg := cassadm.checkValues(test.new)
			require.Equalf(t, test.allowed, allowed, "Wrong value returned from checkValues function. Message: '%s'", msg)
		})
	}
}

func TestCheckTransitions(t *testing.T) {

	old := casstest.NewSimpleCluster(3)
	old.Spec.Datacenter.Racks[0].Storage = rookv1alpha2.StorageScopeSpec{
		Selection: rookv1alpha2.Selection{
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{}},
		},
	}

	versionChanged := old.DeepCopy()
	versionChanged.Spec.Version = "100.100.100"

	repoChanged := old.DeepCopy()
	repoChanged.Spec.Repository = util.RefFromString("my-private-repo")

	modeChanged := old.DeepCopy()
	modeChanged.Spec.Mode = cassandrav1alpha1.ClusterModeScylla

	sidecarImageChanged := old.DeepCopy()
	sidecarImageChanged.Spec.SidecarImage = &cassandrav1alpha1.ImageSpec{
		Version:    "1.0.0",
		Repository: "my-private-repo",
	}

	dcNameChanged := old.DeepCopy()
	dcNameChanged.Spec.Datacenter.Name = "new-random-name"

	rackPlacementChanged := old.DeepCopy()
	rackPlacementChanged.Spec.Datacenter.Racks[0].Placement = &rookv1alpha2.Placement{
		NodeAffinity: &corev1.NodeAffinity{},
	}

	rackStorageChanged := old.DeepCopy()
	rackStorageChanged.Spec.Datacenter.Racks[0].Storage.VolumeClaimTemplates[0].Name = "new-name"

	rackResourcesChanged := old.DeepCopy()
	rackResourcesChanged.Spec.Datacenter.Racks[0].Resources.Requests = map[corev1.ResourceName]resource.Quantity{
		corev1.ResourceCPU: *resource.NewMilliQuantity(1000, resource.DecimalSI),
	}

	rackDeleted := old.DeepCopy()
	rackDeleted.Spec.Datacenter.Racks = nil

	tests := []struct {
		name    string
		new     *cassandrav1alpha1.Cluster
		allowed bool
	}{
		{
			name:    "same as old",
			new:     old,
			allowed: true,
		},
		{
			name:    "version changed",
			new:     versionChanged,
			allowed: false,
		},
		{
			name:    "repo changed",
			new:     repoChanged,
			allowed: false,
		},
		{
			name:    "mode changed",
			new:     modeChanged,
			allowed: false,
		},
		{
			name:    "sidecarImage changed",
			new:     sidecarImageChanged,
			allowed: false,
		},
		{
			name:    "dcName changed",
			new:     dcNameChanged,
			allowed: false,
		},
		{
			name:    "rackPlacement changed",
			new:     rackPlacementChanged,
			allowed: false,
		},
		{
			name:    "rackStorage changed",
			new:     rackStorageChanged,
			allowed: false,
		},
		{
			name:    "rackResources changed",
			new:     rackResourcesChanged,
			allowed: false,
		},
		{
			name:    "rack deleted",
			new:     rackDeleted,
			allowed: false,
		},
	}

	cassadm := &CassandraAdmission{}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, msg := cassadm.checkTransitions(old, test.new)
			require.Equalf(t, test.allowed, allowed, "Wrong value returned from checkTransitions function. Message: '%s'", msg)
		})
	}

}
