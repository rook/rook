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
package k8sutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
)

func TestMakeRookImage(t *testing.T) {
	assert.Equal(t, "rook/rook:v1", MakeRookImage("rook/rook:v1"))
	assert.Equal(t, defaultVersion, MakeRookImage(""))
}

func TestGetContainerInPod(t *testing.T) {
	expectedName := "mycontainer"
	imageName := "myimage"

	// no container fails
	container, err := GetMatchingContainer([]v1.Container{}, expectedName)
	assert.NotNil(t, err)

	// one container will allow any name
	container, err = GetMatchingContainer([]v1.Container{{Name: "foo", Image: imageName}}, expectedName)
	assert.Nil(t, err)
	assert.Equal(t, imageName, container.Image)

	// multiple container fails if we don't find the correct name
	container, err = GetMatchingContainer(
		[]v1.Container{{Name: "foo", Image: imageName}, {Name: "bar", Image: imageName}},
		expectedName)
	assert.NotNil(t, err)

	// multiple container succeeds if we find the correct name
	container, err = GetMatchingContainer(
		[]v1.Container{{Name: "foo", Image: imageName}, {Name: expectedName, Image: imageName}},
		expectedName)
	assert.Nil(t, err)
	assert.Equal(t, imageName, container.Image)
}
