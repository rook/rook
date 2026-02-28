package ttlv

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ansel1/merry"
	"github.com/gemalto/kmip-go/internal/kmiputil"
)

// FormatType formats a byte as a KMIP Type string,
// as described in the KMIP Profiles spec.  If the value is registered,
// the normalized name of the value will be returned.
//
// Otherwise, a 1 byte hex string is returned, but this is not
// technically a valid encoding for types in the JSON and XML encoding
// specs.  Hex values Should only be used for debugging.  Examples:
//
// - Integer
// - 0x42
func FormatType(b byte, enumMap EnumMap) string {
	if enumMap != nil {
		if s, ok := enumMap.Name(uint32(b)); ok {
			return s
		}
	}

	return fmt.Sprintf("%#02x", b)
}

// FormatTag formats an uint32 as a KMIP Tag string,
// as described in the KMIP Profiles spec.  If the value is registered,
// the normalized name of the value will be returned.  Otherwise, a
// 3 byte hex string is returned.  Examples:
//
// - ActivationDate
// - 0x420001
func FormatTag(v uint32, enumMap EnumMap) string {
	if enumMap != nil {
		if s, ok := enumMap.Name(v); ok {
			return s
		}
	}

	return fmt.Sprintf("%#06x", v)
}

// FormatTagCanonical formats an uint32 as a canonical Tag name
// from the KMIP spec.  If the value is registered,
// the canonical name of the value will be returned.  Otherwise, a
// 3 byte hex string is returned.  Examples:
//
// - Activation Date
// - 0x420001
//
// Canonical tag names are used in the AttributeName of Attribute structs.
func FormatTagCanonical(v uint32, enumMap EnumMap) string {
	if enumMap != nil {
		if s, ok := enumMap.CanonicalName(v); ok {
			return s
		}
	}

	return fmt.Sprintf("%#06x", v)
}

// FormatEnum formats an uint32 as a KMIP Enumeration string,
// as described in the KMIP Profiles spec.  If the value is registered,
// the normalized name of the value will be returned.  Otherwise, a
// four byte hex string is returned.  Examples:
//
// - SymmetricKey
// - 0x00000002
func FormatEnum(v uint32, enumMap EnumMap) string {
	if enumMap != nil {
		if s, ok := enumMap.Name(v); ok {
			return s
		}
	}

	return fmt.Sprintf("%#08x", v)
}

// FormatInt formats an integer as a KMIP bitmask string, as
// described in the KMIP Profiles spec for JSON under
// the "Special case for Masks" section.  Examples:
//
// - 0x0000100c
// - Encrypt|Decrypt|CertificateSign
// - CertificateSign|0x00000004|0x0000008
// - CertificateSign|0x0000000c
func FormatInt(i int32, enumMap EnumMap) string {
	if enumMap == nil {
		return fmt.Sprintf("%#08x", i)
	}

	values := enumMap.Values()
	if len(values) == 0 {
		return fmt.Sprintf("%#08x", i)
	}

	v := uint32(i) //nolint:gosec //nolint:gosec

	// bitmask
	// decompose mask into the names of set flags, concatenated by pipe
	// if remaining value (minus registered flags) is not zero, append
	// the remaining value as hex.

	var sb strings.Builder

	for _, v1 := range values {
		if v1&v == v1 {
			if name, ok := enumMap.Name(v1); ok {
				if sb.Len() > 0 {
					sb.WriteString("|")
				}

				sb.WriteString(name)

				v ^= v1
			}
		}

		if v == 0 {
			break
		}
	}

	if v != 0 {
		if sb.Len() > 0 {
			sb.WriteString("|")
		}

		_, _ = fmt.Fprintf(&sb, "%#08x", v)
	}

	return sb.String()
}

// ParseEnum parses a string into a uint32 according to the rules
// in the KMIP Profiles regarding encoding enumeration values.
// See FormatEnum for examples of the formats which can be parsed.
// It will also parse numeric strings.  Examples:
//
//	ParseEnum("UnableToCancel", registry.EnumForTag(TagCancellationResult))
//	ParseEnum("0x00000002")
//	ParseEnum("2")
//
// Returns ErrInvalidHexString if the string is invalid hex, or
// if the hex value is less than 1 byte or more than 4 bytes (ignoring
// leading zeroes).
//
// Returns ErrUnregisteredEnumName if string value is not a
// registered enum value name.
func ParseEnum(s string, enumMap EnumMap) (uint32, error) {
	u, err := strconv.ParseUint(s, 10, 32)
	if err == nil {
		// it was a raw number
		return uint32(u), nil
	}

	v, err := parseHexOrName(s, 4, enumMap)
	if err != nil {
		return 0, merry.Here(err)
	}

	return v, nil
}

// ParseInt parses a string into an int32 according the rules
// in the KMIP Profiles regarding encoding integers, including
// the special rules for bitmasks.  See FormatInt for examples
// of the formats which can be parsed.
//
// Returns ErrInvalidHexString if the string is invalid hex, or
// if the hex value is less than 1 byte or more than 4 bytes (ignoring
// leading zeroes).
//
// Returns ErrUnregisteredEnumName if string value is not a
// registered enum value name.
func ParseInt(s string, enumMap EnumMap) (int32, error) {
	i, err := strconv.ParseInt(s, 10, 32)
	if err == nil {
		// it was a raw number
		return int32(i), nil
	}

	if !strings.ContainsAny(s, "| ") {
		v, err := parseHexOrName(s, 4, enumMap)
		if err != nil {
			return 0, merry.Here(err)
		}

		return int32(v), nil //nolint:gosec
	}

	// split values, look up each, and recombine
	s = strings.ReplaceAll(s, "|", " ")
	parts := strings.Split(s, " ")
	var v uint32

	for _, part := range parts {
		if len(part) == 0 {
			continue
		}

		i, err := parseHexOrName(part, 4, enumMap)
		if err != nil {
			return 0, merry.Here(err)
		}

		v |= i
	}

	return int32(v), nil //nolint:gosec
}

func parseHexOrName(s string, maxLen int, enumMap EnumMap) (uint32, error) {
	b, err := kmiputil.ParseHexValue(s, maxLen)
	if err != nil {
		return 0, err
	}

	if b != nil {
		return kmiputil.DecodeUint32(b), nil
	}

	if enumMap != nil {
		if v, ok := enumMap.Value(s); ok {
			return v, nil
		}
	}

	return 0, merry.Append(ErrUnregisteredEnumName, s)
}

// ParseTag parses a string into Tag according the rules
// in the KMIP Profiles regarding encoding tag values.
// See FormatTag for examples of the formats which can be parsed.
//
// Returns ErrInvalidHexString if the string is invalid hex, or
// if the hex value is less than 1 byte or more than 3 bytes (ignoring
// leading zeroes).
//
// Returns ErrUnregisteredEnumName if string value is not a
// registered enum value name.
func ParseTag(s string, enumMap EnumMap) (Tag, error) {
	v, err := parseHexOrName(s, 3, enumMap)
	if err != nil {
		return 0, merry.Here(err)
	}

	return Tag(v), nil
}

// ParseType parses a string into Type according the rules
// in the KMIP Profiles regarding encoding type values.
// See FormatType for examples of the formats which can be parsed.
// This also supports parsing a hex string type (e.g. "0x01"), though
// this is not technically a valid encoding of a type in the spec.
//
// Returns ErrInvalidHexString if the string is invalid hex, or
// if the hex value is less than 1 byte or more than 3 bytes (ignoring
// leading zeroes).
//
// Returns ErrUnregisteredEnumName if string value is not a
// registered enum value name.
func ParseType(s string, enumMap EnumMap) (Type, error) {
	b, err := kmiputil.ParseHexValue(s, 1)
	if err != nil {
		return 0, merry.Here(err)
	}

	if b != nil {
		return Type(b[0]), nil
	}

	if enumMap != nil {
		if v, ok := enumMap.Value(s); ok {
			return Type(v), nil
		}
	}

	return 0, merry.Here(ErrUnregisteredEnumName).Append(s)
}

// EnumMap defines a set of named enumeration values.  Canonical names should
// be the name from the spec. Names should be in the normalized format
// described in the KMIP spec (see NormalizeName()).
//
// Value enumerations are used for encoding and decoding KMIP Enumeration values,
// KMIP Integer bitmask values, Types, and Tags.
type EnumMap interface {
	// Name returns the normalized name for a value, e.g. AttributeName.
	// If the name is not registered, it returns "", false.
	Name(v uint32) (string, bool)
	// CanonicalName returns the canonical name for the value from the spec,
	// e.g. Attribute Name.
	// If the name is not registered, it returns "", false
	CanonicalName(v uint32) (string, bool)
	// Value returns the value registered for the name argument.  If there is
	// no name registered for this value, it returns 0, false.
	// The name argument may be the canonical name (e.g. "Cryptographic Algorithm") or
	// the normalized name (e.g. "CryptographicAlgorithm").
	Value(name string) (uint32, bool)
	// Values returns the complete set of registered values.  The order
	// they are returned in will be the order they are encoded in when
	// encoding bitmasks as strings.
	Values() []uint32
	// Bitmask returns true if this is an enumeration of bitmask flags.
	Bitmask() bool
}
