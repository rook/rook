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

package k8sutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDeleteResource(t *testing.T) {
	var deleteError error
	stubDelete := func() error { return deleteError }

	k8sNotFoundError := errors.NewNotFound(schema.GroupResource{Group: "group", Resource: "resource"}, "test")

	var verifyCount int
	var verifyReturnDeletedAtCount int
	stubVerify := func() error {
		verifyCount++
		fmt.Println("verifyCount:", verifyCount, "   - returnNilAt:", verifyReturnDeletedAtCount)
		if verifyCount == verifyReturnDeletedAtCount {
			return k8sNotFoundError
		}
		return fmt.Errorf("still not verified")
	}

	defaultWaitOpts := &WaitOptions{RetryCount: 2, RetryInterval: 1 * time.Millisecond}

	// Failure to delete should return error
	deleteError = fmt.Errorf("err on delete")
	err := DeleteResource(stubDelete, stubVerify, "test resource name", &DeleteOptions{}, defaultWaitOpts)
	assert.Error(t, err)

	// NotFound error should return error if DeleteOptions.MustDelete is set
	deleteError = k8sNotFoundError
	err = DeleteResource(stubDelete, stubVerify, "test resource name", &DeleteOptions{MustDelete: true}, defaultWaitOpts)
	assert.Error(t, err)

	// Default behavior should be to continue if resource doesn't exist
	deleteError = k8sNotFoundError
	err = DeleteResource(stubDelete, stubVerify, "test resource name", &DeleteOptions{}, defaultWaitOpts)
	assert.NoError(t, err)

	// Default behavior should be to NOT wait to verify resource
	deleteError = nil
	verifyCount = 0
	err = DeleteResource(stubDelete, stubVerify, "test resource name", &DeleteOptions{}, defaultWaitOpts)
	assert.NoError(t, err)
	assert.Zero(t, verifyCount)

	// Verify that the default wait options are picked up
	// And that the verify function is called one time more than the retry count
	// And that delete can report error on timeout
	deleteError = nil
	verifyCount = 0
	verifyReturnDeletedAtCount = 1000000 // do not return verify success
	deleteOpts := &DeleteOptions{}
	deleteOpts.Wait = true
	deleteOpts.ErrorOnTimeout = true
	err = DeleteResource(stubDelete, stubVerify, "test resource name", deleteOpts, defaultWaitOpts)
	assert.Error(t, err)
	assert.Equal(t, 3, verifyCount)
	assert.Equal(t, 1*time.Millisecond, unitTestRetryIntervalRecord)

	// Test that delete will not report error on timeout if so configured
	deleteOpts.ErrorOnTimeout = false
	err = DeleteResource(stubDelete, stubVerify, "test resource name", deleteOpts, defaultWaitOpts)
	assert.NoError(t, err)

	// Verify that delete will return successfully after some retries
	verifyCount = 0
	verifyReturnDeletedAtCount = 2
	deleteOpts = &DeleteOptions{}
	deleteOpts.Wait = true
	deleteOpts.ErrorOnTimeout = true
	err = DeleteResource(stubDelete, stubVerify, "test resource name", deleteOpts, defaultWaitOpts)
	assert.NoError(t, err)

	// Verify that specified retry count and interval in delete opts are picked up
	deleteError = nil
	verifyCount = 0
	verifyReturnDeletedAtCount = 1000000 // do not return verify success
	deleteOpts = &DeleteOptions{}
	deleteOpts.Wait = true
	deleteOpts.ErrorOnTimeout = true
	deleteOpts.RetryCount = 5
	deleteOpts.RetryInterval = 2 * time.Millisecond
	err = DeleteResource(stubDelete, stubVerify, "test resource name", deleteOpts, defaultWaitOpts)
	assert.Error(t, err)
	assert.Equal(t, 6, verifyCount)
	assert.Equal(t, 2*time.Millisecond, unitTestRetryIntervalRecord)
}
