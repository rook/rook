package kvstore

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	etcdError "github.com/coreos/etcd/error"
)

func GetEtcdClient(listenClientURLs []string) (etcd.KeysAPI, error) {
	if len(listenClientURLs) == 0 {
		return nil, fmt.Errorf("no listen client URLs specified.")
	}

	config := etcd.Config{
		Endpoints:               listenClientURLs,
		Transport:               etcd.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}

	c, err := etcd.New(config)
	if err != nil {
		return nil, err
	}

	return etcd.NewKeysAPI(c), nil
}

// Get all the values for the specified etcd keys, returned as a map of key names to values.
func GetEtcdValues(etcdClient etcd.KeysAPI, keys map[string]string) (values map[string]string, err error) {
	values = map[string]string{}

	for key, etcdKey := range keys {
		result, err := etcdClient.Get(context.Background(), etcdKey, nil)
		if err != nil {
			return nil, err
		}

		values[key] = result.Node.Value
	}

	return
}

// Converts the error to an etcd error code if possible. Returns the code and true if successfully parsed.
func GetEtcdCode(err error) (int, bool) {
	if err != nil {
		if etcdErr, ok := err.(*etcdError.Error); ok {
			return etcdErr.ErrorCode, true
		}
		if strings.Contains(err.Error(), "100: Key not found") {
			return etcdError.EcodeKeyNotFound, true
		}
		if strings.Contains(err.Error(), "105: Key already exists") {
			return etcdError.EcodeNodeExist, true
		}
	}

	return -1, false
}

// Check the error to see if it indicates an etcd key was not found. True if a match, false otherwise.
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
