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

package crash

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCrashEnvVar(t *testing.T) {
	env := generateCrashEnvVar()
	assert.Equal(t, "CEPH_ARGS", env.Name)
	assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST) -k /etc/ceph/crash-collector-keyring-store/keyring", env.Value)
}
