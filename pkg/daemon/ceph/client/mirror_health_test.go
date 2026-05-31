package client

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
)

func TestToCustomResourceStatus(t *testing.T) {
	mirroringStatus := &cephv1.MirroringStatusSummarySpec{}
	mirroringStatus.Health = "HEALTH_OK"
	mirroringInfo := &cephv1.MirroringInfo{
		Mode:     "pool",
		SiteName: "rook-ceph-emea",
		Peers: []cephv1.PeersSpec{
			{UUID: "82656994-3314-4996-ac4c-263c2c9fd081"},
		},
	}

	// Test 1: Empty so it's disabled
	{
		newMirroringStatus, newMirroringInfo, _ := toCustomResourceStatus(&cephv1.MirroringStatusSpec{}, mirroringStatus, &cephv1.MirroringInfoSpec{}, mirroringInfo, &cephv1.SnapshotScheduleStatusSpec{}, []cephv1.SnapshotSchedulesSpec{}, "")
		assert.NotEmpty(t, newMirroringStatus.MirroringStatus)
		assert.Equal(t, "HEALTH_OK", newMirroringStatus.MirroringStatus.Summary.Health)
		assert.Equal(t, "pool", newMirroringInfo.Mode)
	}

	// Test 2: snap sched
	{
		snapSchedStatus := []cephv1.SnapshotSchedulesSpec{
			{
				Pool:  "my-pool",
				Image: "pool/image",
			},
		}
		newMirroringStatus, newMirroringInfo, newSnapshotScheduleStatus := toCustomResourceStatus(&cephv1.MirroringStatusSpec{}, mirroringStatus, &cephv1.MirroringInfoSpec{}, mirroringInfo, &cephv1.SnapshotScheduleStatusSpec{}, snapSchedStatus, "")
		assert.NotEmpty(t, newMirroringStatus.MirroringStatus)
		assert.Equal(t, "HEALTH_OK", newMirroringStatus.MirroringStatus.Summary.Health)
		assert.NotEmpty(t, newMirroringInfo.Mode, "pool")
		assert.NotEmpty(t, newSnapshotScheduleStatus)
	}
}
