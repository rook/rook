package display

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNumToStrOmitEmpty(t *testing.T) {
	assert.Equal(t, "", NumToStrOmitEmpty(0))
	assert.Equal(t, "1", NumToStrOmitEmpty(1))
	assert.Equal(t, "9999", NumToStrOmitEmpty(9999))
	assert.Equal(t, "18446744073709551615", NumToStrOmitEmpty(math.MaxUint64))
}
