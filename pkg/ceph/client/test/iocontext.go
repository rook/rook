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
package test

import "github.com/rook/rook/pkg/ceph/client"

/////////////////////////////////////////////////////////////
// implement the interface for the ceph io context
/////////////////////////////////////////////////////////////
type MockIOContext struct {
	MockGetImageNames func() (names []string, err error)
	MockGetImage      func(name string) client.Image
	MockCreateImage   func(name string, size uint64, order int, args ...uint64) (image client.Image, err error)
}

func (m *MockIOContext) Destroy() {
}

func (m *MockIOContext) Read(oid string, data []byte, offset uint64) (int, error) {
	return 0, nil
}
func (m *MockIOContext) Write(oid string, data []byte, offset uint64) error {
	return nil
}
func (m *MockIOContext) WriteFull(oid string, data []byte) error {
	return nil
}
func (m *MockIOContext) Pointer() uintptr {
	return uintptr(0)
}
func (m *MockIOContext) GetImage(name string) client.Image {
	if m.MockGetImage != nil {
		return m.MockGetImage(name)
	}
	return nil
}
func (m *MockIOContext) GetImageNames() (names []string, err error) {
	if m.MockGetImageNames != nil {
		return m.MockGetImageNames()
	}
	return nil, nil
}
func (m *MockIOContext) CreateImage(name string, size uint64, order int, args ...uint64) (image client.Image, err error) {
	if m.MockCreateImage != nil {
		return m.MockCreateImage(name, size, order, args...)
	}
	return &MockImage{MockName: name}, nil
}
