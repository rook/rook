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

type MockImage struct {
	MockName   string
	MockStat   func() (info *client.ImageInfo, err error)
	MockRemove func() error
}

func (i *MockImage) Open(args ...interface{}) error {
	return nil
}

func (i *MockImage) Close() error {
	return nil
}

func (i *MockImage) Remove() error {
	if i.MockRemove != nil {
		return i.MockRemove()
	}
	return nil
}

func (i *MockImage) Stat() (info *client.ImageInfo, err error) {
	if i.MockStat != nil {
		return i.MockStat()
	}
	return nil, nil
}

func (i *MockImage) Name() string {
	return i.MockName
}
