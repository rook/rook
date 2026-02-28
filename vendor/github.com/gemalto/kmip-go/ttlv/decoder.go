package ttlv

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"reflect"

	"github.com/ansel1/merry"
)

var ErrUnexpectedValue = errors.New("no field was found to unmarshal value into")

// Unmarshal parses TTLV encoded data and stores the result
// in the value pointed to by v.
//
// An error will be returned if v is nil or not a point, or
// if b is not valid TTLV.
//
// Unmarshal will allocate values to store the result in, similar to the
// json.Marshal.  Generally, the destination value can be a pointer or
// or a direct value.  Currently, Unmarshal does not support anonymous fields.
// They will be ignored.  Private fields are ignored.
//
// Unmarshal maps TTLV values to golang values according to the following
// rules:
//
//  1. If the destination value is interface{}, it will be set to the result
//     of TTLV.Value()
//  2. If the destination implements Unmarshaler, that will be called.
//  3. If the destination is a slice (except for []byte), append the
//     unmarshalled value to the slice
//  4. Structure unmarshals into a struct.  See rules
//     below for matching struct fields to the values in the Structure.
//  5. Interval unmarshals into an int64
//  6. DateTime and DateTimeExtended ummarshal into time.Time
//  7. ByteString unmarshals to a []byte
//  8. TextString unmarshals into a string
//  9. Boolean unmarshals into a bool
//  10. Enumeration can unmarshal into an int, int8, int16, int32, or their
//     uint counterparts.  If the KMIP value overflows the destination, a
//     *UnmarshalerError with cause ErrIntOverflow is returned.
//  11. Integer can unmarshal to the same types as Enumeration, with the
//     same overflow check.
//  12. LongInteger unmarshals to int64 or uint64
//  13. BitInteger unmarshals to big.Int.
//
// If the destination value is not a supported type,  an *UnmarshalerError with
// cause ErrUnsupportedTypeError is returned.  If the source value's type is not recognized,
// *UnmarshalerError with cause ErrInvalidType is returned.
//
// # Unmarshaling Structure
//
// Unmarshal will try to match the values in the Structure with the fields in the
// destination struct.  Structure is an array of values, while a struct is more like
// a map, so not all Structure values can be accurately represented by a golang struct.
// In particular, a Structure can hold the same tag multiple times, e.g. 3 TagComment values
// in a row.
//
// For each field in the struct, Unmarshal infers a KMIP Tag by examining both the name
// and type of the field.  It uses the following rules, in order:
//
// 1. If the type of a field is a struct, and the struct contains a field named "TTLVTag", and the field
// has a "ttlv" struct tag, the value of the struct tag will be parsed using ParseTag().  If
// parsing fails, an error is returned.  The type and value of the TTLVTag field is ignored.
//
// In this example, the F field will map to TagDeactivationDate.
//
//	type Bar struct {
//	  F Foo
//	}
//
//	type Foo struct {
//	  TTLVTag struct{} `ttlv:"DeactivationDate"`
//	}
//
// If Bar uses a struct tag on F indicating a different tag, it is an error:
//
//	type Bar struct {
//	  F Foo `ttlv:"DerivationData"`  // this will cause an ErrTagConflict
//	                                 // because conflict Bar's field tag
//	                                 // conflicts with Foo's intrinsic tag
//	  F2 Foo `ttlv:"0x420034"`       // the value can also be hex
//	}
//
// 2. If the type of the field is a struct, and the struct contains a field named "TTLVTag",
// and that field is of type ttlv.Tag and is not empty, the value of the field will be the
// inferred Tag.  For example:
//
//	type Foo struct {
//	  TTLVTag ttlv.Tag
//	}
//	f := Foo{TTLVTag: ttlv.TagState}
//
// This allows you to dynamically set the KMIP tag that a value will marshal to.
//
// 3. The "ttlv" struct tag can be used to indicate the tag for a field.  The value will
// be parsed with ParseTag():
//
//	type Bar struct {
//	  F Foo   `ttlv:"DerivationData"`
//	}
//
// 4. The name of the field is parsed with ParseTag():
//
//	type Bar struct {
//	  DerivationData int
//	}
//
// 5. The name of the field's type is parsed with ParseTab():
//
//	type DerivationData int
//
//	type Bar struct {
//	  dd DerivationData
//	}
//
// If no tag value can be inferred, the field is ignored.  Multiple fields
// *cannot* map to the same KMIP tag.  If they do, an ErrTagConflict will
// be returned.
//
// Each value in the Structure will be matched against the first field
// in the struct with the same inferred tag.
//
// If the value cannot be matched with a field, Unmarshal will look for
// the first field with the "any" struct flag set and unmarshal into that:
//
//	type Foo struct {
//	    Comment string                            // the Comment will unmarshal into this
//	    EverythingElse []interface{}  `,any`      // all other values will unmarshal into this
//	    AnotherAny []interface{}  `,any`          // allowed, but ignored.  first any field will always match
//	    NotLegal []interface{}  `TagComment,any`  // you cannot specify a tag and the any flag.
//	                                              // will return error
//	}
//
// If after applying these rules no destination field is found, the KMIP value is ignored.
func Unmarshal(ttlv TTLV, v interface{}) error {
	return NewDecoder(bytes.NewReader(ttlv)).Decode(v)
}

// Unmarshaler knows how to unmarshal a ttlv value into itself.
// The decoder argument may be used to decode the ttlv value into
// intermediary values if needed.
type Unmarshaler interface {
	UnmarshalTTLV(d *Decoder, ttlv TTLV) error
}

// Decoder reads KMIP values from a stream, and decodes them into golang values.
// It currently only decodes TTLV encoded KMIP values.
// TODO: support decoding XML and JSON, so their decoding can be configured
//
// If DisallowExtraValues is true, the decoder will return an error when decoding
// Structures into structs and a matching field can't get found for every value.
type Decoder struct {
	r                   io.Reader
	bufr                *bufio.Reader
	DisallowExtraValues bool

	currStruct reflect.Type
	currField  string
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		r:    r,
		bufr: bufio.NewReader(r),
	}
}

// Reset resets the internal state of the decoder for reuse.
func (dec *Decoder) Reset(r io.Reader) {
	*dec = Decoder{
		r:    r,
		bufr: dec.bufr,
	}
	dec.bufr.Reset(r)
}

// Decode the first KMIP value from the reader into v.
// See Unmarshal for decoding rules.
func (dec *Decoder) Decode(v interface{}) error {
	ttlv, err := dec.NextTTLV()
	if err != nil {
		return err
	}

	return dec.DecodeValue(v, ttlv)
}

// DecodeValue decodes a ttlv value into v.  This doesn't read anything
// from the Decoder's reader.
// See Unmarshal for decoding rules.
func (dec *Decoder) DecodeValue(v interface{}, ttlv TTLV) error {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Ptr {
		return merry.New("non-pointer passed to Decode")
	}

	return dec.unmarshal(val, ttlv)
}

func (dec *Decoder) unmarshal(val reflect.Value, ttlv TTLV) error {
	if len(ttlv) == 0 {
		return nil
	}

	// Load value from interface, but only if the result will be
	// usefully addressable.
	if val.Kind() == reflect.Interface && !val.IsNil() {
		e := val.Elem()
		if e.Kind() == reflect.Ptr && !e.IsNil() {
			val = e
		}
	}

	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			val.Set(reflect.New(val.Type().Elem()))
		}

		val = val.Elem()
	}

	if val.Type().Implements(unmarshalerType) {
		return val.Interface().(Unmarshaler).UnmarshalTTLV(dec, ttlv) //nolint:forcetypeassert
	}

	if val.CanAddr() {
		valAddr := val.Addr()
		if valAddr.CanInterface() && valAddr.Type().Implements(unmarshalerType) {
			return valAddr.Interface().(Unmarshaler).UnmarshalTTLV(dec, ttlv) //nolint:forcetypeassert
		}
	}

	switch val.Kind() {
	case reflect.Interface:
		if ttlv.Type() == TypeStructure {
			// if the value is a structure, set the whole TTLV
			// as the value.
			val.Set(reflect.ValueOf(ttlv))
		} else {
			// set blank interface equal to the TTLV.Value()
			val.Set(reflect.ValueOf(ttlv.Value()))
		}

		return nil
	case reflect.Slice:
		typ := val.Type()
		if typ.Elem() == byteType {
			// []byte
			break
		}

		// Slice of element values.
		// Grow slice.
		n := val.Len()
		val.Set(reflect.Append(val, reflect.Zero(val.Type().Elem())))

		// Recur to read element into slice.
		if err := dec.unmarshal(val.Index(n), ttlv); err != nil {
			val.SetLen(n)
			return err
		}

		return nil
	default:
	}

	typeMismatchErr := func() error {
		e := &UnmarshalerError{
			Struct: dec.currStruct,
			Field:  dec.currField,
			Tag:    ttlv.Tag(),
			Type:   ttlv.Type(),
			Val:    val.Type(),
		}
		err := merry.WrapSkipping(e, 1).WithCause(ErrUnsupportedTypeError)

		return err
	}

	switch ttlv.Type() {
	case TypeStructure:
		if val.Kind() != reflect.Struct {
			return typeMismatchErr()
		}
		// stash currStruct
		currStruct := dec.currStruct
		err := dec.unmarshalStructure(ttlv, val)
		// restore currStruct
		dec.currStruct = currStruct

		return err
	case TypeInterval:
		if val.Kind() != reflect.Int64 {
			return typeMismatchErr()
		}

		val.SetInt(int64(ttlv.ValueInterval()))
	case TypeDateTime, TypeDateTimeExtended:
		if val.Type() != timeType {
			return typeMismatchErr()
		}

		val.Set(reflect.ValueOf(ttlv.ValueDateTime()))
	case TypeByteString:
		if val.Kind() != reflect.Slice && val.Type().Elem() != byteType {
			return typeMismatchErr()
		}

		val.SetBytes(ttlv.ValueByteString())
	case TypeTextString:
		if val.Kind() != reflect.String {
			return typeMismatchErr()
		}

		val.SetString(ttlv.ValueTextString())
	case TypeBoolean:
		if val.Kind() != reflect.Bool {
			return typeMismatchErr()
		}

		val.SetBool(ttlv.ValueBoolean())
	//nolint:dupl
	case TypeEnumeration:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
			i := int64(ttlv.ValueEnumeration())
			if val.OverflowInt(i) {
				return dec.newUnmarshalerError(ttlv, val.Type(), ErrIntOverflow)
			}

			val.SetInt(i)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
			i := uint64(ttlv.ValueEnumeration())
			if val.OverflowUint(i) {
				return dec.newUnmarshalerError(ttlv, val.Type(), ErrIntOverflow)
			}

			val.SetUint(i)
		default:
			return typeMismatchErr()
		}
	//nolint:dupl
	case TypeInteger:
		switch val.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
			i := int64(ttlv.ValueInteger())
			if val.OverflowInt(i) {
				return dec.newUnmarshalerError(ttlv, val.Type(), ErrIntOverflow)
			}
			val.SetInt(i)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
			i := uint64(ttlv.ValueInteger()) //nolint:gosec // already prevented by the check above
			if val.OverflowUint(i) {
				return dec.newUnmarshalerError(ttlv, val.Type(), ErrIntOverflow)
			}

			val.SetUint(i)
		default:
			return typeMismatchErr()
		}
	case TypeLongInteger:
		switch val.Kind() {
		case reflect.Int64:
			i := ttlv.ValueLongInteger()
			if val.OverflowInt(i) {
				return dec.newUnmarshalerError(ttlv, val.Type(), ErrIntOverflow)
			}
			val.SetInt(i)
		case reflect.Uint64:
			i := uint64(ttlv.ValueLongInteger()) //nolint:gosec // already prevented by the check above
			if val.OverflowUint(i) {
				return dec.newUnmarshalerError(ttlv, val.Type(), ErrUintOverflow)
			}
			val.SetUint(i)
		default:
			return typeMismatchErr()
		}
	case TypeBigInteger:
		if val.Type() != bigIntType {
			return typeMismatchErr()
		}

		val.Set(reflect.ValueOf(*ttlv.ValueBigInteger()))
	default:
		return dec.newUnmarshalerError(ttlv, val.Type(), ErrInvalidType)
	}

	return nil
}

func (dec *Decoder) unmarshalStructure(ttlv TTLV, val reflect.Value) error {
	ti, err := getTypeInfo(val.Type())
	if err != nil {
		return dec.newUnmarshalerError(ttlv, val.Type(), err)
	}

	if ti.tagField != nil && ti.tagField.ti.typ == tagType {
		val.FieldByIndex(ti.tagField.index).Set(reflect.ValueOf(ttlv.Tag()))
	}

	fields := ti.valueFields

	// push currStruct (caller will pop)
	dec.currStruct = val.Type()

	for n := ttlv.ValueStructure(); n != nil; n = n.Next() {
		fldIdx := -1

		for i := range fields {
			if fields[i].flags.any() {
				// if this is the first any field found, keep track
				// of it as the current candidate match, but
				// keep looking for a tag match
				if fldIdx == -1 {
					fldIdx = i
				}
			} else if fields[i].tag == n.Tag() {
				// tag match found
				// we can stop looking
				fldIdx = i
				break
			}
		}

		if fldIdx > -1 {
			// push currField
			currField := dec.currField
			dec.currField = fields[fldIdx].name
			err := dec.unmarshal(val.FieldByIndex(fields[fldIdx].index), n)
			// restore currField
			dec.currField = currField

			if err != nil {
				return err
			}
		} else if dec.DisallowExtraValues {
			return dec.newUnmarshalerError(ttlv, val.Type(), ErrUnexpectedValue)
		}
	}

	return nil
}

// NextTTLV reads the next, full KMIP value off the reader.
func (dec *Decoder) NextTTLV() (TTLV, error) {
	// first, read the header
	header, err := dec.bufr.Peek(8)
	if err != nil {
		return nil, merry.Wrap(err)
	}

	if err := TTLV(header).ValidHeader(); err != nil {
		// bad header, abort
		return TTLV(header), merry.Prependf(err, "invalid header: %v", TTLV(header))
	}

	// allocate a buffer large enough for the entire message
	fullLen := TTLV(header).FullLen()
	buf := make([]byte, fullLen)

	var totRead int

	for {
		n, err := dec.bufr.Read(buf[totRead:])
		if err != nil {
			return TTLV(buf), merry.Wrap(err)
		}

		totRead += n
		if totRead >= fullLen {
			// we've read off a single full message
			return buf, nil
		} // else keep reading
	}
}

func (dec *Decoder) newUnmarshalerError(ttlv TTLV, valType reflect.Type, cause error) merry.Error {
	e := &UnmarshalerError{
		Struct: dec.currStruct,
		Field:  dec.currField,
		Tag:    ttlv.Tag(),
		Type:   ttlv.Type(),
		Val:    valType,
	}

	return merry.WrapSkipping(e, 1).WithCause(cause)
}

type UnmarshalerError struct {
	// Val is the type of the destination value
	Val reflect.Type
	// Struct is the type of the containing struct if the value is a field
	Struct reflect.Type
	// Field is the name of the value field
	Field string
	Tag   Tag
	Type  Type
}

func (e *UnmarshalerError) Error() string {
	msg := "kmip: error unmarshaling " + e.Tag.String() + " with type " + e.Type.String() + " into value of type " + e.Val.Name()
	if e.Struct != nil {
		msg += " in struct field " + e.Struct.Name() + "." + e.Field
	}

	return msg
}
