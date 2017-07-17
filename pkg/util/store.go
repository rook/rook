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
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	etcdError "github.com/coreos/etcd/error"
	ctx "golang.org/x/net/context"
)

const (
	etcdServersEnvVar = "CLUSTERD_ETCD_SERVERS"
	InfiniteTimeout   = -1
)

// Store each of the properties in etcd
func StoreEtcdProperties(etcdClient etcd.KeysAPI, baseKey string, properties map[string]string) error {

	failedCount := 0
	var lastError error = nil
	for key, value := range properties {
		_, err := etcdClient.Set(ctx.Background(), path.Join(baseKey, key), value, nil)
		if err != nil {
			failedCount++
			lastError = err
		}
	}

	if failedCount > 0 {
		logger.Errorf("Failed to update %d properties in %s", failedCount, baseKey)
		return lastError
	}
	return nil
}

// Watch an etcd key for changes since the provided index
func WatchEtcdKey(etcdClient etcd.KeysAPI, key string, index *uint64, timeout int) (string, bool, error) {
	options := &etcd.WatcherOptions{AfterIndex: *index}
	watcher := etcdClient.Watcher(key, options)
	cancelableContext, cancelFunc := ctx.WithCancel(ctx.Background())

	value := ""
	var err error = nil
	watcherChannel := make(chan bool, 1)
	go func() {
		logger.Tracef("waiting for response")
		var response *etcd.Response
		response, err = watcher.Next(cancelableContext)
		if err != nil {
			if err != ctx.Canceled {
				// If there was an error watching, attempt to get the value of the key and reset the current index
				// This can be a common occurrence for the index to get out of date. See documentation on the error
				// "The event in requested index is outdated and cleared"
				response, geterr := etcdClient.Get(ctx.Background(), key, nil)
				if geterr == nil {
					logger.Infof("Watching %s failed on index %d, but Get succeeded with index %d", key, *index, response.Index)
					*index = response.Index
					value = response.Node.Value
					err = nil
				}
			}
		} else {
			logger.Tracef("Watched key %s, value=%s, index=%d", key, response.Node.Value, *index)
			*index = response.Index
			value = response.Node.Value
		}
		watcherChannel <- true
	}()

	if timeout == InfiniteTimeout {
		// Wait indefinitely for the etcd watcher to respond
		logger.Tracef("Watching key %s after index %d", key, *index)
		<-watcherChannel
		return value, false, err

	} else {
		// Start a timer to allow a timeout if the watch doesn't return in a timely manner
		timer := time.NewTimer(time.Second * time.Duration(timeout))

		// Return when the first channel completes
		logger.Infof("Watching key %s after index %d for at most %d seconds", key, *index, timeout)
		select {
		case <-timer.C:
			logger.Warningf("Timed out watching key %s", key)
			cancelFunc()
			return "", true, errors.New("the etcd watch timed out")
		case <-watcherChannel:
			logger.Infof("Completed watching key %s. value=%s", key, value)
			timer.Stop()
			return value, false, err
		}
	}
}

// Create an etcd key. Ignores the error that it already exists.
func CreateEtcdDir(etcdClient etcd.KeysAPI, key string) error {
	_, err := etcdClient.Set(ctx.Background(), key, "", &etcd.SetOptions{Dir: true, PrevExist: etcd.PrevNoExist})
	if err != nil && IsEtcdNodeExist(err) {
		return nil
	}

	return err
}

func GetEtcdClientFromEnv() (etcd.KeysAPI, error) {
	peers, err := GetEtcdPeers()
	if err != nil {
		return nil, err
	}

	return GetEtcdClient(peers)
}

func GetEtcdClient(peers []string) (etcd.KeysAPI, error) {

	config := etcd.Config{
		Endpoints:               peers,
		Transport:               etcd.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}

	c, err := etcd.New(config)
	if err != nil {
		return nil, err
	}

	return etcd.NewKeysAPI(c), nil
}

func GetEtcdPeers() ([]string, error) {
	etcdPeers := os.Getenv(etcdServersEnvVar)
	if etcdPeers == "" {
		return nil, errors.New(etcdServersEnvVar + " is not set")
	}

	machines := strings.Split(etcdPeers, ",")
	logger.Debugf("etcd machines: %v", machines)
	return machines, nil
}

// Get all the values for the specified etcd keys, returned as a map of key names to values.
func GetEtcdValues(etcdClient etcd.KeysAPI, keys map[string]string) (values map[string]string, err error) {
	values = map[string]string{}

	for key, etcdKey := range keys {
		result, err := etcdClient.Get(ctx.Background(), etcdKey, nil)
		if err != nil {
			return nil, err
		}

		values[key] = result.Node.Value
	}

	return
}

func GetDirChildKeys(etcdClient etcd.KeysAPI, key string) (*Set, error) {
	children := NewSet()
	resp, err := etcdClient.Get(context.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if IsEtcdKeyNotFound(err) {
			// The key was not found. Return the empty set
			return children, nil
		}

		return children, err
	}

	if resp != nil {
		if resp.Node.Dir {
			for _, child := range resp.Node.Nodes {
				children.Add(GetLeafKeyPath(child.Key))
			}

			return children, nil
		}
	}

	return children, nil
}

func GetLeafKeyPath(key string) string {
	lastIndex := strings.LastIndex(key, "/")
	if lastIndex == -1 {
		// path separator not found, return entire key
		return key
	}

	// return the leaf key, which is everything from right after the last path separator to the
	// end of the key
	return key[lastIndex+1:]
}

func GetParentKeyPath(key string) string {
	// get the last index of the path separator
	lastIndex := strings.LastIndex(key, "/")
	if lastIndex == -1 {
		// no path separator found, there is no parent for this key
		return ""
	}

	// return the parent key, which would be everything up to the last path separator
	return key[0:lastIndex]
}

// Converts the error to an etcd error code if possible. Returns the code and true if successfully parsed.
func GetEtcdCode(err error) (int, bool) {
	if etcdErr, ok := err.(*etcdError.Error); ok {
		return etcdErr.ErrorCode, true
	}
	if strings.Contains(err.Error(), "100: Key not found") {
		return etcdError.EcodeKeyNotFound, true
	}
	if strings.Contains(err.Error(), "105: Key already exists") {
		return etcdError.EcodeNodeExist, true
	}

	return -1, false
}

func EtcdDirExists(etcdClient etcd.KeysAPI, key string) (bool, error) {
	val, err := etcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if IsEtcdKeyNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return val != nil, nil
}

// Check the error to see if it indicates an etcd was not found. True if a match, false otherwise.
func IsEtcdKeyNotFound(err error) bool {
	if code, ok := GetEtcdCode(err); ok && code == etcdError.EcodeKeyNotFound {
		return true
	}

	return false
}

// Check the error to see if it indicates an etcd key already exists.  True if so, false otherwise.
func IsEtcdNodeExist(err error) bool {
	if code, ok := GetEtcdCode(err); ok && code == etcdError.EcodeNodeExist {
		return true
	}

	return false
}

// Check if the error indicates the etcd key was reset
func IsEtcdKeyReset(err error) bool {
	if code, ok := GetEtcdCode(err); ok && (code == etcdError.EcodeEventIndexCleared || code == etcdError.EcodeWatcherCleared) {
		return true
	}

	return false
}

func IsEtcdDirAlreadyExists(err error) bool {
	if err != nil {
		if etcdErr, ok := err.(etcd.Error); ok {
			if etcdErr.Code == etcdError.EcodeNotFile {
				// etcd uses "Not a file" error code when you try to set a dir that already exists
				return true
			}
		}
	}

	return false
}

func IsEtcdCompareFailed(err error) bool {
	if err != nil {
		if etcdErr, ok := err.(etcd.Error); ok {
			if etcdErr.Code == etcdError.EcodeTestFailed {
				return true
			}
		}
	}

	return false
}
