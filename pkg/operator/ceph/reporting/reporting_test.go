/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Reporting focuses on reporting Events, Status Conditions, and the like to users.
package reporting

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func Test_objKindOrBestGuess(t *testing.T) {
	var nilCephCluster *cephv1.CephCluster = nil

	tests := []struct {
		name  string
		inObj client.Object
		want  string
	}{
		{"CephCluster with API", &cephv1.CephCluster{
			TypeMeta: metav1.TypeMeta{
				Kind: "CephCluster",
			},
		}, "CephCluster"},
		{"CephCluster by inference", &cephv1.CephCluster{}, "CephCluster"},
		{"CephCluster with wrong API", &cephv1.CephCluster{
			TypeMeta: metav1.TypeMeta{
				Kind: "WRONG",
			},
		}, "WRONG"},
		{"untyped nil", nil, unknownKind},
		{"nil CephCluster", nilCephCluster, "CephCluster"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, objKindOrBestGuess(tt.inObj))
		})
	}
}

func TestReportReconcileResult(t *testing.T) {
	setupTest := func() (*capnslog.PackageLogger, *bytes.Buffer, *record.FakeRecorder) {
		logBuf := bytes.NewBuffer([]byte{})
		logFmt := capnslog.NewLogFormatter(logBuf, "", 0)
		capnslog.SetFormatter(logFmt)
		logger := capnslog.NewPackageLogger("github.com/rook/rook", "")
		capnslog.SetGlobalLogLevel(capnslog.TRACE)

		recorder := record.NewFakeRecorder(3)

		return logger, logBuf, recorder
	}

	name := "my-cluster"
	namespace := "rook-ceph"

	cephCluster := &cephv1.CephCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CephCluster",
			APIVersion: cephv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	reconcileRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}

	successMsg := `successfully configured CephCluster "rook-ceph/my-cluster"`
	successEvent := `Normal ReconcileSucceeded ` + successMsg

	fakeErr := errors.New("fake-err")
	errorMsg := `failed to reconcile CephCluster "rook-ceph/my-cluster". fake-err`
	errorEvent := `Warning ReconcileFailed ` + errorMsg

	t.Run("successful reconcile", func(t *testing.T) {
		logger, logBuf, recorder := setupTest()

		result, err := ReportReconcileResult(logger, recorder, reconcileRequest,
			cephCluster, reconcile.Result{}, nil)
		assert.NoError(t, err)
		assert.True(t, result.IsZero())
		assert.Equal(t, successMsg, strings.TrimSpace(logBuf.String()))
		assert.Len(t, recorder.Events, 1)
		assert.Equal(t, successEvent, <-recorder.Events)
	})

	t.Run("reconcile with error", func(t *testing.T) {
		logger, logBuf, recorder := setupTest()

		result, err := ReportReconcileResult(logger, recorder, reconcileRequest,
			cephCluster, reconcile.Result{}, fakeErr)
		assert.Error(t, err)
		assert.True(t, result.IsZero())
		assert.Equal(t, errorMsg, strings.TrimSpace(logBuf.String()))
		assert.Len(t, recorder.Events, 1)
		assert.Equal(t, errorEvent, <-recorder.Events)
	})

	t.Run("reconcile with requeue", func(t *testing.T) {
		logger, logBuf, recorder := setupTest()

		inResult := reconcile.Result{Requeue: true}
		result, err := ReportReconcileResult(logger, recorder, reconcileRequest,
			cephCluster, inResult, nil)
		assert.NoError(t, err)
		assert.Equal(t, inResult, result)
		assert.Equal(t, successMsg, strings.TrimSpace(logBuf.String()))
		assert.Len(t, recorder.Events, 1)
		assert.Equal(t, successEvent, <-recorder.Events)
	})

	t.Run("failed reconcile with requeue", func(t *testing.T) {
		logger, logBuf, recorder := setupTest()

		inResult := reconcile.Result{RequeueAfter: 567 * time.Second}
		result, err := ReportReconcileResult(logger, recorder, reconcileRequest,
			cephCluster, inResult, fakeErr)
		// this is the trick: when we get a request to requeue after a time _AND_ an error as input,
		// we do not propagate the error to the output, _BUT_ we still record the error in log/event.
		// this is so we can still report errors to users, but the controller-runtime will still
		// obey our desire to reconcile with a delay
		assert.NoError(t, err)
		assert.Equal(t, inResult, result)
		assert.Equal(t, errorMsg, strings.TrimSpace(logBuf.String()))
		assert.Len(t, recorder.Events, 1)
		assert.Equal(t, errorEvent, <-recorder.Events)
	})

	t.Run("success with empty object", func(t *testing.T) {
		// success returning an empty object might be because a reconcile request is for an
		// already-deleted object

		logger, logBuf, recorder := setupTest()

		result, err := ReportReconcileResult(logger, recorder, reconcileRequest,
			&cephv1.CephCluster{}, reconcile.Result{}, nil)
		assert.NoError(t, err)
		assert.True(t, result.IsZero())
		assert.Equal(t, successMsg, strings.TrimSpace(logBuf.String()))
		assert.Len(t, recorder.Events, 1)
		assert.Equal(t, successEvent, <-recorder.Events)
	})

	t.Run("failure with nil object", func(t *testing.T) {
		// failure with a nil object might be because a reconcile returned nil for the object after
		// an error accidentally

		logger, logBuf, recorder := setupTest()

		var obj *cephv1.CephCluster = nil

		result, err := ReportReconcileResult(logger, recorder, reconcileRequest,
			obj, reconcile.Result{}, fakeErr)
		assert.Error(t, err)
		assert.True(t, result.IsZero())
		assert.Contains(t, logBuf.String(), `object associated with reconcile request CephCluster "rook-ceph/my-cluster" should not be nil`)
		assert.Contains(t, logBuf.String(), errorMsg)
		assert.Len(t, recorder.Events, 1)
		assert.Equal(t, errorEvent, <-recorder.Events)
	})
}
