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

type MockKeyValueStore struct {
	Stores map[string]map[string]string
}

func NewMockKeyValueStore() *MockKeyValueStore {
	return &MockKeyValueStore{
		Stores: make(map[string]map[string]string),
	}
}

func (m *MockKeyValueStore) GetValue(storeName, key string) (string, error) {
	store, ok := m.Stores[storeName]
	if !ok {
		return "", NewNotExistError(storeName, key)
	}

	val, ok := store[key]
	if !ok {
		return "", NewNotExistError(storeName, key)
	}

	return val, nil
}

func (m *MockKeyValueStore) SetValue(storeName, key, value string) error {
	store, ok := m.Stores[storeName]
	if !ok {
		// the given store name doesn't exist yet, create it now
		store = make(map[string]string)
		m.Stores[storeName] = store
	}

	store[key] = value
	return nil
}

func (m *MockKeyValueStore) GetStore(storeName string) (map[string]string, error) {
	store, ok := m.Stores[storeName]
	if !ok {
		return nil, NewNotExistError(storeName, "")
	}

	return store, nil
}

func (m *MockKeyValueStore) ClearStore(storeName string) error {
	delete(m.Stores, storeName)
	return nil
}
