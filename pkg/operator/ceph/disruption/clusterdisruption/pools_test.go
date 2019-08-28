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

package clusterdisruption

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

func TestGetMinimumFailureDomain(t *testing.T) {
	poolList := []cephv1.PoolSpec{
		{FailureDomain: "region"},
		{FailureDomain: "zone"},
	}

	assert.Equal(t, "zone", getMinimumFailureDomain(poolList))

	poolList = []cephv1.PoolSpec{
		{FailureDomain: "region"},
		{FailureDomain: "zone"},
		{FailureDomain: "host"},
	}

	assert.Equal(t, "host", getMinimumFailureDomain(poolList))

	// test default
	poolList = []cephv1.PoolSpec{
		{FailureDomain: "aaa"},
		{FailureDomain: "bbb"},
		{FailureDomain: "ccc"},
	}

	assert.Equal(t, "host", getMinimumFailureDomain(poolList))

}
