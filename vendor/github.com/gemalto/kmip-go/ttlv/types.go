package ttlv

import (
	"io"
	"time"
)

func RegisterTypes(r *Registry) {
	m := map[string]Type{
		"BigInteger":       TypeBigInteger,
		"Boolean":          TypeBoolean,
		"ByteString":       TypeByteString,
		"DateTime":         TypeDateTime,
		"Enumeration":      TypeEnumeration,
		"Integer":          TypeInteger,
		"Interval":         TypeInterval,
		"LongInteger":      TypeLongInteger,
		"Structure":        TypeStructure,
		"TextString":       TypeTextString,
		"DateTimeExtended": TypeDateTimeExtended,
	}

	for name, v := range m {
		r.RegisterType(v, name)
	}
}

// Type describes the type of a KMIP TTLV.
// 2 and 9.1.1.2
type Type byte

const (
	TypeStructure        Type = 0x01
	TypeInteger          Type = 0x02
	TypeLongInteger      Type = 0x03
	TypeBigInteger       Type = 0x04
	TypeEnumeration      Type = 0x05
	TypeBoolean          Type = 0x06
	TypeTextString       Type = 0x07
	TypeByteString       Type = 0x08
	TypeDateTime         Type = 0x09
	TypeInterval         Type = 0x0A
	TypeDateTimeExtended Type = 0x0B
)

// String returns the normalized name of the type.  If the type
// name isn't registered, it returns the hex value of the type,
// e.g. "0x01" (TypeStructure).  The value of String() is suitable
// for use in the JSON or XML encoding of TTLV.
func (t Type) String() string {
	return DefaultRegistry.FormatType(t)
}

func (t Type) MarshalText() (text []byte, err error) {
	return []byte(t.String()), nil
}

func (t *Type) UnmarshalText(text []byte) (err error) {
	*t, err = DefaultRegistry.ParseType(string(text))
	return
}

// DateTimeExtended is a time wrapper which always marshals to a DateTimeExtended.
type DateTimeExtended struct {
	time.Time
}

func (t *DateTimeExtended) UnmarshalTTLV(d *Decoder, ttlv TTLV) error {
	if len(ttlv) == 0 {
		return nil
	}

	if t == nil {
		*t = DateTimeExtended{}
	}

	err := d.DecodeValue(&t.Time, ttlv)
	if err != nil {
		return err
	}

	return nil
}

func (t DateTimeExtended) MarshalTTLV(e *Encoder, tag Tag) error {
	e.EncodeDateTimeExtended(tag, t.Time)
	return nil
}

// Value is a go-typed mapping for a TTLV value.  It holds a tag, and the value in
// the form of a native go type.
//
// Value supports marshaling and unmarshaling, allowing a mapping between encoded TTLV
// bytes and native go types.  It's useful in tests, or where you want to construct
// an arbitrary TTLV structure in code without declaring a bespoke type, e.g.:
//
//	v := ttlv.Value{
//	       Tag: TagBatchCount, Value: Values{
//	         Value{Tag: TagComment, Value: "red"},
//	         Value{Tag: TagComment, Value: "blue"},
//	         Value{Tag: TagComment, Value: "green"},
//	       }
//	t, err := ttlv.Marshal(v)
//
// KMIP Structure types are mapped to the Values go type.  When marshaling, if the Value
// field is set to a Values{}, the resulting TTLV will be TypeStructure.  When unmarshaling
// a TTLV with TypeStructure, the Value field will be set to a Values{}.
type Value struct {
	Tag   Tag
	Value interface{}
}

// UnmarshalTTLV implements Unmarshaler
func (t *Value) UnmarshalTTLV(d *Decoder, ttlv TTLV) error {
	t.Tag = ttlv.Tag()

	switch ttlv.Type() {
	case TypeStructure:
		var v Values

		ttlv = ttlv.ValueStructure()
		for ttlv.Valid() == nil {
			err := d.DecodeValue(&v, ttlv)
			if err != nil {
				return err
			}

			ttlv = ttlv.Next()
		}

		t.Value = v
	default:
		t.Value = ttlv.Value()
	}

	return nil
}

// MarshalTTLV implements Marshaler
func (t Value) MarshalTTLV(e *Encoder, tag Tag) error {
	// if tag is set, override the suggested tag
	if t.Tag != TagNone {
		tag = t.Tag
	}

	if tvs, ok := t.Value.(Values); ok {
		return e.EncodeStructure(tag, func(e *Encoder) error {
			for _, v := range tvs {
				if err := e.Encode(v); err != nil {
					return err
				}
			}

			return nil
		})
	}

	return e.EncodeValue(tag, t.Value)
}

// Values is a slice of Value objects.  It represents the body of a TTLV with a type of Structure.
type Values []Value

// NewValue creates a new tagged value.
func NewValue(tag Tag, val interface{}) Value {
	return Value{
		Tag:   tag,
		Value: val,
	}
}

// NewStruct creates a new tagged value which is of type struct.
func NewStruct(tag Tag, vals ...Value) Value {
	return Value{
		Tag:   tag,
		Value: Values(vals),
	}
}

type Encoder struct {
	encodeDepth int
	w           io.Writer
	encBuf      encBuf

	// these fields store where the encoder is when marshaling a nested struct.  its
	// used to construct error messages.
	currStruct string
	currField  string
}

// EnumValue is a uint32 wrapper which always encodes as an enumeration.
type EnumValue uint32

func (v EnumValue) MarshalTTLV(e *Encoder, tag Tag) error {
	e.EncodeEnumeration(tag, uint32(v))
	return nil
}
