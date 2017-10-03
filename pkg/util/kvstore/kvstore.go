/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package kvstore

import "fmt"

type KeyValueStore interface {
	GetValue(storeName, key string) (string, error)
	SetValue(storeName, key, value string) error
	GetStore(storeName string) (map[string]string, error)
	ClearStore(storeName string) error
}

type NotExistError struct {
	StoreName string
	KeyName   string
}

func NewNotExistError(storeName, key string) *NotExistError {
	return &NotExistError{
		StoreName: storeName,
		KeyName:   key,
	}
}

func (e *NotExistError) Error() string {
	return fmt.Sprintf("Key %s does not exist in store %s", e.KeyName, e.StoreName)
}

func IsNotExist(err error) bool {
	_, ok := err.(*NotExistError)
	return ok
}
