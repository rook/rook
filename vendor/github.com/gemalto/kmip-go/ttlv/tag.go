package ttlv

const (
	TagNone               = Tag(0)
	tagAttributeName  Tag = 0x42000a
	tagAttributeValue Tag = 0x42000b
)

// Tag
// 9.1.3.1
type Tag uint32

// String returns the normalized name of the tag.
func (t Tag) String() string {
	return DefaultRegistry.FormatTag(t)
}

// CanonicalName returns the canonical name of the tag.
func (t Tag) CanonicalName() string {
	return DefaultRegistry.FormatTagCanonical(t)
}

func (t Tag) MarshalText() (text []byte, err error) {
	return []byte(t.String()), nil
}

func (t *Tag) UnmarshalText(text []byte) (err error) {
	*t, err = DefaultRegistry.ParseTag(string(text))
	return
}

const (
	minStandardTag uint32 = 0x00420000
	maxStandardTag uint32 = 0x00430000
	minCustomTag   uint32 = 0x00540000
	maxCustomTag   uint32 = 0x00550000
)

// Valid checks whether the tag's numeric value is valid according to
// the ranges in the spec.
func (t Tag) Valid() bool {
	switch {
	case uint32(t) >= minStandardTag && uint32(t) < maxStandardTag:
		return true
	case uint32(t) >= minCustomTag && uint32(t) < maxCustomTag:
		return true
	default:
		return false
	}
}
