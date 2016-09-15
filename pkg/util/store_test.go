package util

import (
	"errors"
	"os"
	"testing"

	etcdError "github.com/coreos/etcd/error"
	"github.com/stretchr/testify/assert"
)

func TestGetEtcdClient(t *testing.T) {
	peers, err := GetEtcdPeers()
	assert.Nil(t, peers)
	assert.NotNil(t, err)

	os.Setenv("CLUSTERD_ETCD_SERVERS", "foo,bar")
	peers, err = GetEtcdPeers()
	assert.Equal(t, 2, len(peers))
	assert.Equal(t, "foo", peers[0])
	assert.Equal(t, "bar", peers[1])
	assert.Nil(t, err)

	etcdClient, err := GetEtcdClientFromEnv()
	assert.Nil(t, err)
	assert.NotNil(t, etcdClient)
}

func TestGetParentKeyPath(t *testing.T) {
	testGetParentKeyPath(t, "/parent/123", "/parent")
	testGetParentKeyPath(t, "/parent/123/456", "/parent/123")
	testGetParentKeyPath(t, "/parent", "")
	testGetParentKeyPath(t, "", "")
	testGetParentKeyPath(t, "NoPathSeparator", "")
}

func testGetParentKeyPath(t *testing.T, key string, expected string) {
	result := GetParentKeyPath(key)
	assert.Equal(t, expected, result)
}

func TestGetLeafKeyPath(t *testing.T) {
	testGetLeafKeyPath(t, "/foo/bar/baz", "baz")
	testGetLeafKeyPath(t, "/foo", "foo")
	testGetLeafKeyPath(t, "baz", "baz")
	testGetLeafKeyPath(t, "", "")
}

func testGetLeafKeyPath(t *testing.T, key string, expected string) {
	result := GetLeafKeyPath(key)
	assert.Equal(t, expected, result)
}

func TestEtcdErrorCode(t *testing.T) {
	// Be able to parse the etcdError type
	etcdErr := etcdError.NewError(etcdError.EcodeInvalidField, "test", 1)
	e, parsed := GetEtcdCode(etcdErr)
	assert.True(t, parsed)
	assert.Equal(t, etcdError.EcodeInvalidField, e)

	// Parse some error codes based on the error message even when the etcd error type is not being interpreted
	err := errors.New("100: Key not found. where did it go?")
	e, parsed = GetEtcdCode(err)
	assert.True(t, parsed)
	assert.Equal(t, etcdError.EcodeKeyNotFound, e)

	err = errors.New("105: Key already exists. why are you creating it again?")
	e, parsed = GetEtcdCode(err)
	assert.True(t, parsed)
	assert.Equal(t, etcdError.EcodeNodeExist, e)

	// Not parsed
	err = errors.New("110: Don't understand this error")
	e, parsed = GetEtcdCode(err)
	assert.False(t, parsed)
	assert.Equal(t, -1, e)
}
