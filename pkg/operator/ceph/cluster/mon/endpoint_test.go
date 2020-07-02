/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package mon

import (
	"testing"

	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
)

func TestMonFlattening(t *testing.T) {

	// single endpoint
	mons := map[string]*cephclient.MonInfo{
		"foo": {Name: "foo", Endpoint: "1.2.3.4:5000"},
	}
	flattened := FlattenMonEndpoints(mons)
	assert.Equal(t, "foo=1.2.3.4:5000", flattened)
	parsed := ParseMonEndpoints(flattened)
	assert.Equal(t, 1, len(parsed))
	assert.Equal(t, "foo", parsed["foo"].Name)
	assert.Equal(t, "1.2.3.4:5000", parsed["foo"].Endpoint)

	// multiple endpoints
	mons["bar"] = &cephclient.MonInfo{Name: "bar", Endpoint: "2.3.4.5:6000"}
	flattened = FlattenMonEndpoints(mons)
	parsed = ParseMonEndpoints(flattened)
	assert.Equal(t, 2, len(parsed))
	assert.Equal(t, "foo", parsed["foo"].Name)
	assert.Equal(t, "1.2.3.4:5000", parsed["foo"].Endpoint)
	assert.Equal(t, "bar", parsed["bar"].Name)
	assert.Equal(t, "2.3.4.5:6000", parsed["bar"].Endpoint)
}
