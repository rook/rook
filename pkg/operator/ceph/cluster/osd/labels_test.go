/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package osd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOSDTopologyLabels(t *testing.T) {
	fakeLocation := "root=default host=ocs-deviceset-gp2-1-data-0-wh5wl region=us-east-1 zone=us-east-1c"
	result := getOSDTopologyLocationLabels(fakeLocation)
	assert.Equal(t, "us-east-1", result["topology-location-region"])
	assert.Equal(t, "ocs-deviceset-gp2-1-data-0-wh5wl", result["topology-location-host"])
	assert.Equal(t, "us-east-1c", result["topology-location-zone"])
}
