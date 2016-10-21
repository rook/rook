package sys

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindUUID(t *testing.T) {
	output := `Creating new GPT entries.
Disk /dev/sdb: 20971520 sectors, 10.0 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): 922B1335-4C65-4478-A965-EB7652EF6C80
Partition table holds up to 128 entries
First usable sector is 34, last usable sector is 20971486
Partitions will be aligned on 2048-sector boundaries
Total free space is 20971453 sectors (10.0 GiB)
`
	uuid, err := parseUUID("sdb", output)
	assert.Nil(t, err)
	assert.Equal(t, "922b1335-4c65-4478-a965-eb7652ef6c80", uuid)
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
