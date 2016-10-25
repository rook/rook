package sys

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindUUID(t *testing.T) {
	output := `Disk /dev/sdb: 10485760 sectors, 5.0 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): 31273B25-7B2E-4D31-BAC9-EE77E62EAC71
Partition table holds up to 128 entries
First usable sector is 34, last usable sector is 10485726
Partitions will be aligned on 2048-sector boundaries
Total free space is 20971453 sectors (10.0 GiB)
`
	uuid, err := parseUUID("sdb", output)
	assert.Nil(t, err)
	assert.Equal(t, "31273b25-7b2e-4d31-bac9-ee77e62eac71", uuid)
}

func TestParseFileSystem(t *testing.T) {
	output := `Filesystem     Type
devtmpfs       devtmpfs
/dev/sda9      ext4
/dev/sda3      ext4
/dev/sda1      vfat
tmpfs          tmpfs
tmpfs          tmpfs
/dev/sda6      ext4
sdc            tmpfs`

	result := parseDFOutput("sda", output)
	assert.Equal(t, "ext4,ext4,vfat,ext4", result)

	result = parseDFOutput("sdb", output)
	assert.Equal(t, "", result)

	result = parseDFOutput("sdc", output)
	assert.Equal(t, "", result)
}
