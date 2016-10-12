package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimMachineID(t *testing.T) {
	testTrimMachineID(t, " 123 		", "123")
	testTrimMachineID(t, " 1234567890", "1234567890")
	testTrimMachineID(t, " 123456789012", "123456789012")
	testTrimMachineID(t, " 1234567890123", "123456789012")
	testTrimMachineID(t, "1234567890123", "123456789012")
	testTrimMachineID(t, "123456789012345678", "123456789012")
}

func testTrimMachineID(t *testing.T, input, expected string) {
	result := trimMachineID(input)
	assert.Equal(t, expected, result)
}
