/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package cluster to manage Kubernetes storage.
package cluster

import (
	"reflect"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/stretchr/testify/assert"
)

func TestCephStatus(t *testing.T) {
	newStatus := &client.CephStatus{
		Health: client.HealthStatus{Status: "HEALTH_OK"},
	}

	// Empty initial status will have no previous health
	currentStatus := cephv1.ClusterStatus{}
	aggregateStatus := toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, "", aggregateStatus.LastChanged)
	assert.Equal(t, "", aggregateStatus.PreviousHealth)
	assert.Equal(t, 0, len(aggregateStatus.Details))

	// Set the current status to the same as the new status and there will be no previous health
	currentStatus.CephStatus = &cephv1.CephStatus{
		Health: "HEALTH_OK",
	}
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, "", aggregateStatus.LastChanged)
	assert.Equal(t, "", aggregateStatus.PreviousHealth)
	assert.Equal(t, 0, len(aggregateStatus.Details))

	// Set the new status to something different and we should get a previous health
	// Simulate the previous check a minute ago.
	previousTime := formatTime(time.Now().Add(-time.Minute).UTC())
	currentStatus.CephStatus.LastChecked = previousTime
	newStatus.Health.Status = "HEALTH_WARN"
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_WARN", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, aggregateStatus.LastChecked, aggregateStatus.LastChanged)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.PreviousHealth)
	assert.Equal(t, 0, len(aggregateStatus.Details))

	// Add some details to the warning
	osdDownMsg := client.CheckMessage{Severity: "HEALTH_WARN"}
	osdDownMsg.Summary.Message = "1 osd down"
	pgAvailMsg := client.CheckMessage{Severity: "HEALTH_ERR"}
	pgAvailMsg.Summary.Message = "'Reduced data availability: 100 pgs stale'"
	newStatus.Health.Checks = map[string]client.CheckMessage{
		"OSD_DOWN":        osdDownMsg,
		"PG_AVAILABILITY": pgAvailMsg,
	}
	newStatus.Health.Status = "HEALTH_ERR"
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_ERR", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, aggregateStatus.LastChecked, aggregateStatus.LastChanged)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.PreviousHealth)
	assert.Equal(t, 2, len(aggregateStatus.Details))
	assert.Equal(t, osdDownMsg.Summary.Message, aggregateStatus.Details["OSD_DOWN"].Message)
	assert.Equal(t, osdDownMsg.Severity, aggregateStatus.Details["OSD_DOWN"].Severity)
	assert.Equal(t, pgAvailMsg.Summary.Message, aggregateStatus.Details["PG_AVAILABILITY"].Message)
	assert.Equal(t, pgAvailMsg.Severity, aggregateStatus.Details["PG_AVAILABILITY"].Severity)
}

func Test_cephStatusChecker_conditionMessageReason(t *testing.T) {
	c := &clusterd.Context{}
	ns := "rook-ceph"
	type fields struct {
		context      *clusterd.Context
		namespace    string
		resourceName string
		interval     time.Duration
		externalCred config.ExternalCred
		isExternal   bool
	}
	type args struct {
		condition cephv1.ConditionType
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   cephv1.ConditionType
		want1  string
		want2  string
	}{
		{"failure-converged", fields{c, ns, ns, defaultStatusCheckInterval, config.ExternalCred{}, false}, args{cephv1.ConditionFailure}, cephv1.ConditionFailure, "ClusterFailure", "Failed to configure ceph cluster"},
		{"failure-external", fields{c, ns, ns, defaultStatusCheckInterval, config.ExternalCred{}, true}, args{cephv1.ConditionFailure}, cephv1.ConditionFailure, "ClusterFailure", "Failed to configure external ceph cluster"},
		{"success-converged", fields{c, ns, ns, defaultStatusCheckInterval, config.ExternalCred{}, false}, args{cephv1.ConditionReady}, cephv1.ConditionReady, "ClusterCreated", "Cluster created successfully"},
		{"success-external", fields{c, ns, ns, defaultStatusCheckInterval, config.ExternalCred{}, true}, args{cephv1.ConditionReady}, cephv1.ConditionConnected, "ClusterConnected", "Cluster connected successfully"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cephStatusChecker{
				context:      tt.fields.context,
				namespace:    tt.fields.namespace,
				resourceName: tt.fields.resourceName,
				interval:     tt.fields.interval,
				externalCred: tt.fields.externalCred,
				isExternal:   tt.fields.isExternal,
			}
			got, got1, got2 := c.conditionMessageReason(tt.args.condition)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("cephStatusChecker.conditionMessageReason() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("cephStatusChecker.conditionMessageReason() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("cephStatusChecker.conditionMessageReason() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}
