package util

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytesToString(t *testing.T) {
	// one value for each unit
	assert.Equal(t, "1 B", BytesToString(1))
	assert.Equal(t, "1.00 KiB", BytesToString(1024))
	assert.Equal(t, "2.22 MiB", BytesToString(2327839))
	assert.Equal(t, "3.33 GiB", BytesToString(3575560274))
	assert.Equal(t, "4.44 TiB", BytesToString(4881831627325))
	assert.Equal(t, "5.55 PiB", BytesToString(6248744482976563))
	assert.Equal(t, "6.66 EiB", BytesToString(7678457220681600860))

	// min and max values
	assert.Equal(t, "0 B", BytesToString(0))
	assert.Equal(t, "16.00 EiB", BytesToString(math.MaxUint64))
}
