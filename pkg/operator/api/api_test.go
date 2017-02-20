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
package api

import (
	"os"
	"testing"

	"github.com/rook/rook/pkg/operator/api"
	"github.com/stretchr/testify/assert"
)

func TestMakeRookOperatorImage(t *testing.T) {
	assert.Equal(t, "quay.io/rook/rook-operator:v1", api.MakeRookOperatorImage("v1"))
}

func TestMakeRookOperatorImageWithEnv(t *testing.T) {
	os.Setenv("ROOK_OPERATOR_REPO_PREFIX", "myrepo.io/rook")
	assert.Equal(t, "myrepo.io/rook/rook-operator:v1", api.MakeRookOperatorImage("v1"))
	os.Setenv("ROOK_OPERATOR_REPO_PREFIX", "")
}
