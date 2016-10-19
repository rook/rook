package partition

import (
	"log"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleScheme(t *testing.T) {
	scheme, err := GetSimpleScheme(123)
	assert.Nil(t, err)
	assert.Equal(t, 123, scheme.SizeMB)
	assert.Equal(t, 36, len(scheme.DiskUUID))
	assert.Equal(t, 3, len(scheme.PartitionUUIDs))

	args := scheme.GetArgs("foo")
	assert.Equal(t, 11, len(args))

	err = scheme.Save("/tmp")
	defer os.Remove(path.Join("/tmp", schemeFilename))
	assert.Nil(t, err)

	loaded, err := LoadScheme("/tmp")
	assert.Nil(t, err)
	assert.Equal(t, 3, len(loaded.PartitionUUIDs))

	log.Printf("scheme=%+v", scheme)
}
