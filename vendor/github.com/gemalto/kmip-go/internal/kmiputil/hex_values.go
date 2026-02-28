package kmiputil

import (
	"encoding/binary"
	"encoding/hex"
	"strings"

	"github.com/ansel1/merry"
)

var ErrInvalidHexString = merry.New("invalid hex string")

func DecodeUint32(b []byte) uint32 {
	// pad to 4 bytes with leading zeros
	return binary.BigEndian.Uint32(pad(b, 4))
}

func DecodeUint64(b []byte) uint64 {
	// pad to 8 bytes with leading zeros
	return binary.BigEndian.Uint64(pad(b, 8))
}

func pad(b []byte, l int) []byte {
	if len(b) < l {
		b2 := make([]byte, l)
		copy(b2[l-len(b):], b)
		b = b2
	}

	return b
}

// ParseHexValue attempts to parse a string formatted as a hex value
// as described in the KMIP Profiles spec, in the "Hex representations" section.
//
// If the string doesn't start with the required prefix "0x", it is assumed the string
// is not a hex representation, and nil, nil is returned.
//
// An ErrInvalidHexString is returned if the hex parsing fails.
// If the maxLen argument is >0, ErrInvalidHexString is returned if the number of bytes parsed
// is greater than maxLen, ignoring leading zeros.  All bytes parsed are returned (including
// leading zeros).
func ParseHexValue(s string, maxLen int) ([]byte, error) {
	if !strings.HasPrefix(s, "0x") {
		return nil, nil
	}

	b, err := hex.DecodeString(s[2:])
	if err != nil {
		return nil, merry.WithCause(ErrInvalidHexString, err).Append(err.Error())
	}

	if maxLen > 0 {
		l := len(b)
		// minus leading zeros
		for i := 0; i < len(b) && b[i] == 0; i++ {
			l--
		}

		if l > maxLen {
			return nil, merry.Appendf(ErrInvalidHexString, "must be %v bytes", maxLen)
		}
	}

	return b, nil
}
