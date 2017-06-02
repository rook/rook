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
package client

// interface for the ceph io context
type IOContext interface {
	Destroy()
	Read(oid string, data []byte, offset uint64) (int, error)
	Write(oid string, data []byte, offset uint64) error
	WriteFull(oid string, data []byte) error
	Pointer() uintptr
	GetImage(name string) Image
	GetImageNames() (names []string, err error)
	CreateImage(name string, size uint64, order int, args ...uint64) (image Image, err error)
}
