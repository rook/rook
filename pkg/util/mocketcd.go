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
package util

import (
	"errors"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"

	etcd "github.com/coreos/etcd/client"
	etcdError "github.com/coreos/etcd/error"
	"golang.org/x/net/context"
)

// ************************************************************************************************
// KeysAPI interface mock implementation
// ************************************************************************************************
type MockEtcdClient struct {
	MockSet          func(ctx context.Context, key, value string, opts *etcd.SetOptions) (*etcd.Response, error)
	MockGet          func(ctx context.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error)
	MockDelete       func(ctx context.Context, key string, opts *etcd.DeleteOptions) (*etcd.Response, error)
	MockWatcher      func(key string, opts *etcd.WatcherOptions) etcd.Watcher
	WatcherResponses map[string]string
	store            *MockEtcdDir
	index            uint64
	watchCount       int
}

type MockEtcdDir struct {
	Path   string
	Dirs   map[string]*MockEtcdDir
	Values map[string]string
}

func NewMockEtcdClient() *MockEtcdClient {
	client := &MockEtcdClient{}
	client.ResetStore()
	return client
}

// Get an etcd value. Defaults to an in-memory response to values that have been set for this client.
func (r *MockEtcdClient) Get(ctx context.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
	if r.MockGet != nil {
		resp, err := r.MockGet(ctx, key, opts)
		if resp != nil || err != nil {
			return resp, err
		}
	}
	return r.memoryGet(ctx, key, opts)
}

// Set an etcd value. Defaults to an in-memory store.
func (r *MockEtcdClient) Set(ctx context.Context, key, value string, opts *etcd.SetOptions) (*etcd.Response, error) {
	r.index++
	if r.MockSet != nil {
		resp, err := r.MockSet(ctx, key, value, opts)
		if resp != nil || err != nil {
			return resp, err
		}
	}
	return r.memorySet(ctx, key, value, opts)
}

func (r *MockEtcdClient) Delete(ctx context.Context, key string, opts *etcd.DeleteOptions) (*etcd.Response, error) {
	if r.MockDelete != nil {
		return r.MockDelete(ctx, key, opts)
	}

	parent, child, err := r.store.getParentDir(key)
	if err != nil {
		// Ignore when a key to delete does not exist
		return nil, nil
	}

	delete(parent.Values, child)

	if opts != nil {
		if opts.Dir {
			delete(parent.Dirs, child)
		}
		if opts.Recursive {
			// How is this different than deleting the dir? It's already cleaned up implicitly
		}
	}
	return nil, nil
}

func (r *MockEtcdClient) Create(ctx context.Context, key, value string) (*etcd.Response, error) {
	return nil, nil
}

func (r *MockEtcdClient) CreateInOrder(ctx context.Context, dir, value string, opts *etcd.CreateInOrderOptions) (*etcd.Response, error) {
	return nil, nil
}

func (r *MockEtcdClient) Update(ctx context.Context, key, value string) (*etcd.Response, error) {
	return nil, nil
}

func (r *MockEtcdClient) Watcher(key string, opts *etcd.WatcherOptions) etcd.Watcher {
	if r.MockWatcher != nil {
		return r.MockWatcher(key, opts)
	} else if r.WatcherResponses != nil && len(r.WatcherResponses) > 0 {
		r.watchCount++
		if r.watchCount > 500 {
			r.Dump()
			panic(fmt.Sprintf("I feel overworked watching etcd so fast. key=%s", key))
		}

		if response, ok := r.WatcherResponses[key]; ok {
			return &MockWatcher{
				MockNext: func(ctx context.Context) (*etcd.Response, error) {
					return MockEtcdValueResponse(key, response, 1), nil
				},
			}
		} else {
			// If the test is specifying WatcherResponses, the key must be set
			panic(fmt.Sprintf("key %s was not found in the mock WatcherResponses", key))
		}
	}

	// default implementation is to just return a watcher that cancels right away
	return &MockWatcher{
		MockNext: func(ctx context.Context) (*etcd.Response, error) {
			return nil, context.Canceled
		},
	}
}

// Reset the test value and directory store
func (r *MockEtcdClient) ResetStore() {
	r.store = &MockEtcdDir{Path: "/"}
	if r.WatcherResponses == nil {
		r.WatcherResponses = make(map[string]string)
	}
}

func (r *MockEtcdClient) Dump() {
	if r.store == nil {
		r.ResetStore()
	}

	r.store.dumpSortedSubdirs()
	r.store.dumpSortedValues()
}

func (d *MockEtcdDir) dumpSortedSubdirs() {
	log.Printf("%s  (%d subkeys, %d values)", d.Path, len(d.Dirs), len(d.Values))

	// first make a copy of the keys from the dir store so we can sort it and not affect the original
	dirs := make([]string, len(d.Dirs))
	i := 0
	for dir, _ := range d.Dirs {
		dirs[i] = dir
		i++
	}

	// now sort our copy and then print out the dirs
	sort.Strings(dirs)
	for _, dir := range dirs {
		// Recursively print the values
		subdir, _ := d.Dirs[dir]
		subdir.dumpSortedSubdirs()
		subdir.dumpSortedValues()
	}
}

func (d *MockEtcdDir) dumpSortedValues() {
	// first make a copy of the keys to not affect the original
	keys := make([]string, len(d.Values))
	i := 0
	for k, _ := range d.Values {
		keys[i] = k
		i++
	}

	// now sort our copy and then print out the key/value pairs
	sort.Strings(keys)
	for _, key := range keys {
		log.Printf("%s/%s=%s", d.Path, key, d.Values[key])
	}
}

// Get the child directories of the key
func (r *MockEtcdClient) GetChildDirs(key string) *Set {
	children, err := GetDirChildKeys(r, key)
	if err != nil {
		log.Printf("failed to get children of key %s. err=%v", key, err)
		return nil
	}
	return children
}

// Get a value. If the key was not found, returns the empty string.
// To distinguish between an empty value and a key not found, call Get()
func (r *MockEtcdClient) GetValue(key string) string {
	response, err := r.memoryGet(context.Background(), key, nil)
	if err != nil {
		return ""
	}
	return response.Node.Value
}

// Set a value
func (r *MockEtcdClient) SetValue(key, value string) {
	r.memorySet(context.Background(), key, value, nil)
}

// Delete a value
func (r *MockEtcdClient) DeleteValue(key string) {
	r.Delete(context.Background(), key, nil)
}

// Delete a key and its children
func (r *MockEtcdClient) DeleteDir(key string) {
	r.Delete(context.Background(), key, &etcd.DeleteOptions{Dir: true})
}

// Create a key and its parents as needed
func (r *MockEtcdClient) CreateDir(key string) {
	r.Set(context.Background(), key, "", &etcd.SetOptions{Dir: true})
}

func (r *MockEtcdClient) CreateDirs(key string, children *Set) {
	for child := range children.Iter() {
		r.CreateDir(path.Join(key, child))
	}
}

func (r *MockEtcdClient) memoryGet(ctx context.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
	if r.store == nil {
		r.store = &MockEtcdDir{Path: "/"}
	}

	parent, child, err := r.store.getParentDir(key)
	if err == nil {
		//log.Printf("memoryGet: parent=%s, child=%s, values=%d, dirs=%d", parent.Path, child, len(parent.Values), len(parent.Dirs))
		if existingValue, ok := parent.Values[child]; ok {
			return MockEtcdValueResponse(key, existingValue, r.index), nil
		}

		recursive := opts != nil && opts.Recursive
		if key == "/" {
			// Returning the root directory
			return r.MockEtcdDirResponse(key, parent, recursive, r.index), nil
		} else if childDir, ok := parent.Dirs[child]; ok {
			// Returning child directories
			return r.MockEtcdDirResponse(key, childDir, recursive, r.index), nil
		}
	}

	return nil, etcdError.NewError(etcdError.EcodeKeyNotFound, "value not found", r.index)
}

// Set a value in the memory store
func (r *MockEtcdClient) memorySet(ctx context.Context, key, value string, opts *etcd.SetOptions) (*etcd.Response, error) {
	// Initialize the map on first use
	if r.store == nil {
		r.ResetStore()
	}

	parent, child, err := r.store.getParentDir(key)

	// Check if the value already exists
	if err == nil {
		if existingValue, ok := parent.Values[child]; ok {
			if opts != nil && opts.PrevExist == etcd.PrevNoExist {
				return MockEtcdValueResponse(key, existingValue, r.index), errors.New("value already set")
			}
		}
	}

	// Set the value
	if opts != nil && opts.Dir {
		if _, ok := parent.Dirs[child]; ok {
			if opts.PrevExist == etcd.PrevNoExist {
				return nil, etcdError.NewError(etcdError.EcodeNodeExist, "dir already exists", r.index)
			}
		}
		// Add the directory and all its parents
		r.addDirectory(key)
	} else {
		// Add the key's parent(s)
		parentKey, child := splitParentKey(key)
		if parentKey == "" {
			parent = r.store
		} else {
			parent = r.addDirectory(parentKey)
		}

		// Add the value
		if parent.Values == nil {
			parent.Values = make(map[string]string)
		}

		parent.Values[child] = value
	}
	return MockEtcdValueResponse(key, value, r.index), nil
}

func (d *MockEtcdDir) getParentDir(key string) (parent *MockEtcdDir, child string, err error) {
	// Trim the leading slash
	key = strings.TrimPrefix(key, "/")

	segments := strings.Split(key, "/")
	parent = d
	err = nil
	if len(segments) == 0 {
		child = ""
		return
	}
	if len(segments) == 1 {
		child = segments[0]
		return
	}

	// The leaf is the last segment of the key
	child = segments[len(segments)-1]

	// Traverse the tree to find the last dir
	for i, segment := range segments {
		if i == len(segments)-1 {
			child = segment
			// The last segment should be ignored since it's the child
			break
		}
		dir, ok := parent.Dirs[segment]
		if !ok {
			err = errors.New("key not found: " + key)
			return
		}
		parent = dir
	}

	return
}

func splitParentKey(key string) (parent, child string) {

	childIndex := strings.LastIndex(key, "/")
	parent = key[0:childIndex]
	child = key[childIndex+1 : len(key)]
	return
}

func splitFirstParentKey(key string) (parent, child string) {
	key = strings.TrimPrefix(key, "/")
	childIndex := strings.Index(key, "/")
	if childIndex < 0 {
		parent = key
		child = ""
		return
	}

	parent = key[0:childIndex]
	child = key[childIndex+1 : len(key)]
	return
}

// Add a directory to etcd and ensure all its parents are created as needed
func (r *MockEtcdClient) addDirectory(key string) *MockEtcdDir {
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		return r.store
	}

	return r.store.addDirectory(key)
}

func (d *MockEtcdDir) addDirectory(key string) *MockEtcdDir {
	if key == "" {
		// The recursion bottomed out
		return d
	}

	child, recursiveChildren := splitFirstParentKey(key)

	// Check if the directory already exists
	if dir, ok := d.Dirs[child]; ok {
		return dir.addDirectory(recursiveChildren)
	}

	// Create the directory
	childDir := &MockEtcdDir{Path: path.Join(d.Path, child), Dirs: make(map[string]*MockEtcdDir), Values: make(map[string]string)}
	if d.Dirs == nil {
		d.Dirs = make(map[string]*MockEtcdDir)
	}
	d.Dirs[child] = childDir

	return childDir.addDirectory(recursiveChildren)
}

// ************************************************************************************************
// Helpers to create etcd mocked responses
// ************************************************************************************************
func (r *MockEtcdClient) MockEtcdDirResponse(key string, dir *MockEtcdDir, recursive bool, index uint64) *etcd.Response {
	return &etcd.Response{
		Node: &etcd.Node{
			Key:   key,
			Dir:   true,
			Nodes: dir.getChildNodes(recursive),
		},
		Index: index,
	}
}

func (d *MockEtcdDir) getChildNodes(recursive bool) etcd.Nodes {

	// Initialize the etcd nodes collection that will contain them all
	childNodes := make(etcd.Nodes, len(d.Dirs)+len(d.Values))
	childIndex := 0

	// create and add etcd nodes for each child value
	for childKey, childValue := range d.Values {
		childNode := &etcd.Node{
			Key:   path.Join(d.Path, childKey),
			Value: childValue,
			Dir:   false,
		}

		childNodes[childIndex] = childNode
		childIndex++
	}

	// create and add etcd nodes for each child dir
	for name, childDir := range d.Dirs {

		childNode := &etcd.Node{
			Key: path.Join(d.Path, name),
			Dir: true,
		}

		childNode.Nodes = childDir.getChildNodes(recursive)

		childNodes[childIndex] = childNode
		childIndex++
	}

	return childNodes
}

func MockEtcdValueResponse(key string, value string, index uint64) *etcd.Response {
	return &etcd.Response{
		Node: &etcd.Node{
			Key:   key,
			Dir:   false,
			Value: value,
		},
		Index: index,
	}
}

// ************************************************************************************************
// Watcher interface Mock implementation
// ************************************************************************************************
type MockWatcher struct {
	MockNext func(context.Context) (*etcd.Response, error)
}

func (r *MockWatcher) Next(ctx context.Context) (*etcd.Response, error) {
	if r.MockNext != nil {
		return r.MockNext(ctx)
	}

	return nil, nil
}
