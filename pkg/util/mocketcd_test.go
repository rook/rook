package util

import (
	"testing"

	etcd "github.com/coreos/etcd/client"
	"golang.org/x/net/context"

	"github.com/stretchr/testify/assert"
)

func TestCreateEtcdDir(t *testing.T) {
	etcdClient := NewMockEtcdClient()
	err := CreateEtcdDir(etcdClient, "/foo/mydir")
	assert.Nil(t, err)

	exists, err := EtcdDirExists(etcdClient, "/foo/mydir")
	assert.Nil(t, err)
	assert.True(t, exists)

	exists, err = EtcdDirExists(etcdClient, "/foo/nodir")
	assert.Nil(t, err)
	assert.False(t, exists)
}

func TestGetChildNodes(t *testing.T) {
	etcdClient := NewMockEtcdClient()
	etcdClient.SetValue("/a/b", "12")
	etcdClient.SetValue("/a/c", "23")

	rootNode := etcdClient.store.getChildNodes(true)
	assert.Equal(t, 1, len(rootNode))
	assert.Equal(t, "/a", rootNode[0].Key)
	values := rootNode[0].Nodes
	assert.Equal(t, 2, len(values))
	set := CreateSet([]string{values[0].Value, values[1].Value})
	assert.True(t, set.Contains("12"))
	assert.True(t, set.Contains("23"))
}

func TestGetValue(t *testing.T) {
	etcdClient := NewMockEtcdClient()

	etcdClient.Set(context.Background(), "/a/b", "12", &etcd.SetOptions{})
	etcdClient.Set(context.Background(), "/a/c", "23", &etcd.SetOptions{})

	val := etcdClient.GetValue("/a/b")
	assert.Equal(t, "12", val)

	val = etcdClient.GetValue("/a/c")
	assert.Equal(t, "23", val)

	val = etcdClient.GetValue("/a/d")
	assert.Equal(t, "", val)

	// Set a root value
	etcdClient.Set(context.Background(), "/myroot", "abc", nil)
	assert.Equal(t, "abc", etcdClient.GetValue("/myroot"))
}

func TestGetParentDir(t *testing.T) {
	etcdClient := NewMockEtcdClient()
	etcdClient.Set(context.Background(), "/a/b/c/d", "", &etcd.SetOptions{Dir: true})
	etcdClient.Set(context.Background(), "/a/b/f", "", &etcd.SetOptions{Dir: true})

	parent, child, err := etcdClient.store.getParentDir("/a")
	assert.Nil(t, err)
	assert.Equal(t, 1, len(parent.Dirs))
	assert.Equal(t, "a", child)
	if _, ok := parent.Dirs[child]; !ok {
		assert.Fail(t, "missing child a")
	}

	parent, child, err = etcdClient.store.getParentDir("/a/b")
	assert.Nil(t, err)
	assert.Equal(t, 1, len(parent.Dirs))
	assert.Equal(t, "b", child)
	if _, ok := parent.Dirs[child]; !ok {
		assert.Fail(t, "missing child b")
	}
}

func TestMockEtcdSetDir(t *testing.T) {
	etcdClient := NewMockEtcdClient()
	etcdClient.Set(context.Background(), "/a/b/c/d", "", &etcd.SetOptions{Dir: true})
	etcdClient.Set(context.Background(), "/e", "", &etcd.SetOptions{Dir: true})

	assert.True(t, etcdClient.GetChildDirs("/a/b/c").Contains("d"))
	assert.True(t, etcdClient.GetChildDirs("/a/b").Contains("c"))
	assert.True(t, etcdClient.GetChildDirs("/a").Contains("b"))
	root := etcdClient.GetChildDirs("/")
	assert.True(t, root.Contains("a"))
	assert.True(t, root.Contains("e"))
	assert.Equal(t, 2, root.Count())

	etcdClient.Set(context.Background(), "/a/b/c/e", "", &etcd.SetOptions{Dir: true})
	etcdClient.Set(context.Background(), "/a/b/c/f", "", &etcd.SetOptions{Dir: true})
	siblings := etcdClient.GetChildDirs("/a/b/c")
	assert.Equal(t, 3, siblings.Count())
	assert.True(t, siblings.Contains("d"))
	assert.True(t, siblings.Contains("e"))
	assert.True(t, siblings.Contains("f"))
}

func TestGetChildDirs(t *testing.T) {
	etcdClient := NewMockEtcdClient()
	etcdClient.CreateDir("/rook/foo/bar")
	etcdClient.CreateDir("/rook/foo/baz")

	children := etcdClient.GetChildDirs("/rook/foo")
	assert.NotNil(t, children)
	assert.Equal(t, 2, children.Count())
	assert.True(t, children.Contains("bar"))
	assert.True(t, children.Contains("baz"))

	children = etcdClient.GetChildDirs("/rook/notfound")
	assert.NotNil(t, children)
	assert.Equal(t, 0, children.Count())
}

func TestEtcdDelete(t *testing.T) {
	etcdClient := NewMockEtcdClient()
	etcdClient.Set(context.Background(), "/a/b/c/d", "value", &etcd.SetOptions{Dir: false})
	etcdClient.Delete(context.Background(), "/a/b/c", &etcd.DeleteOptions{Dir: true, Recursive: true})

	// The values and dirs are not found anymore
	val := etcdClient.GetValue("/a/b/c/d")
	assert.Equal(t, "", val)
	children := etcdClient.GetChildDirs("/a/b")
	assert.Equal(t, 0, children.Count())
	children = etcdClient.GetChildDirs("/a")
	assert.Equal(t, 1, children.Count())
}

func TestEtcdWatch(t *testing.T) {
	etcdClient := NewMockEtcdClient()

	// Return a context canceled error if there is no value to watch for
	watcher := etcdClient.Watcher("/my/notfound", nil)
	val, err := watcher.Next(context.Background())
	assert.Nil(t, val)
	assert.Equal(t, context.Canceled, err)

	// Return the value when specified
	etcdClient.WatcherResponses["/my/value"] = "23"
	watcher = etcdClient.Watcher("/my/value", nil)
	val, err = watcher.Next(context.Background())
	assert.Equal(t, "23", val.Node.Value)
	assert.Nil(t, err)

}
