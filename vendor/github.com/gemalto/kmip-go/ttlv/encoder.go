package ttlv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/ansel1/merry"
)

const structFieldTag = "ttlv"

var (
	ErrIntOverflow              = fmt.Errorf("value exceeds max int value %d", math.MaxInt32)
	ErrUintOverflow             = fmt.Errorf("value exceeds max uint value %d", math.MaxUint32)
	ErrUnsupportedEnumTypeError = errors.New("unsupported type for enums, must be string, or int types")
	ErrUnsupportedTypeError     = errors.New("marshaling/unmarshaling is not supported for this type")
	ErrNoTag                    = errors.New("unable to determine tag for field")
	ErrTagConflict              = errors.New("tag conflict")
)

// Marshal encodes a golang value into a KMIP value.
//
// An error will be returned if v is an invalid pointer.
//
// Currently, Marshal does not support anonymous fields.
// Private fields are ignored.
//
// Marshal maps the golang value to a KMIP tag, type, and value
// encoding.  To determine the KMIP tag, Marshal uses the same rules
// as Unmarshal.
//
// The appropriate type and encoding are inferred from the golang type
// and from the inferred KMIP tag, according to these rules:
//
// 1. If the value is a TTLV, it is copied byte for byte
//
// 2. If the value implements Marshaler, call that
//
// 3. If the struct field has an "omitempty" flag, and the value is
// zero, skip the field:
//
//	type Foo struct {
//	  Comment string `ttlv:,omitempty`
//	}
//
// 4. If the value is a slice (except []byte)  or array, marshal all
// values concatenated
//
// 5. If a tag has not been inferred at this point, return *MarshalerError with
// cause ErrNoTag
//
// 6. If the Tag is registered as an enum, or has the "enum" struct tag flag, attempt
// to marshal as an Enumeration.  int, int8, int16, int32, and their uint counterparts
// can be marshaled as an Enumeration.  A string can be marshaled to an Enumeration
// if the string contains a number, a 4 byte (8 char) hex string with the prefix "0x",
// or the normalized name of an enum value registered to this tag.  Examples:
//
//	type Foo struct {
//	  CancellationResult string    // will encode as an Enumeration because
//	                               // the tag CancellationResult is registered
//	                               // as an enum.
//	  C int `ttlv:"Comment,enum"   // The tag Comment is not registered as an enum
//	                               // but the enum flag will force this to encode
//	                               // as an enumeration.
//	}
//
// If the string can't be interpreted as an enum value, it will be encoded as a TextString.  If
// the "enum" struct flag is set, the value *must* successfully encode to an Enumeration using
// above rules, or an error is returned.
//
// 7. If the Tag is registered as a bitmask, or has the "bitmask" struct tag flag, attempt
// to marshal to an Integer, following the same rules as for Enumerations.  The ParseInt()
// function is used to parse string values.
//
// 9. time.Time marshals to DateTime.  If the field has the "datetimeextended" struct flag,
// marshal as DateTimeExtended.  Example:
//
//	type Foo struct {
//	  ActivationDate time.Time  `ttlv:",datetimeextended"`
//	}
//
// 10. big.Int marshals to BigInteger
//
// 11. time.Duration marshals to Interval
//
// 12. string marshals to TextString
//
// 13. []byte marshals to ByteString
//
// 14. all int and uint variants except int64 and uint64 marshal to Integer.  If the golang
// value overflows the KMIP value, *MarshalerError with cause ErrIntOverflow is returned
//
// 15. int64 and uint64 marshal to LongInteger
//
// 16. bool marshals to Boolean
//
// 17. structs marshal to Structure.  Each field of the struct will be marshaled into the
// values of the Structure according to the above rules.
//
// Any other golang type will return *MarshalerError with cause ErrUnsupportedTypeError.
func Marshal(v interface{}) (TTLV, error) {
	buf := bytes.NewBuffer(nil)

	err := NewEncoder(buf).Encode(v)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Marshaler knows how to encode itself to TTLV.
// The implementation should use the primitive methods of the encoder,
// such as EncodeInteger(), etc.
//
// The tag inferred by the Encoder from the field or type information is
// passed as an argument, but the implementation can choose to ignore it.
type Marshaler interface {
	MarshalTTLV(e *Encoder, tag Tag) error
}

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode a single value and flush to the writer.  The tag will be inferred from
// the value.  If no tag can be inferred, an error is returned.
// See Marshal for encoding rules.
func (e *Encoder) Encode(v interface{}) error {
	return e.EncodeValue(TagNone, v)
}

// EncodeValue encodes a single value with the given tag and flushes it
// to the writer.
// See Marshal for encoding rules.
func (e *Encoder) EncodeValue(tag Tag, v interface{}) error {
	err := e.encode(tag, reflect.ValueOf(v), nil)
	if err != nil {
		return err
	}

	return e.Flush()
}

// EncodeStructure encodes a Structure with the given tag to the writer.
// The function argument should encode the enclosed values inside the Structure.
// Call Flush() to write the data to the writer.
func (e *Encoder) EncodeStructure(tag Tag, f func(e *Encoder) error) error {
	e.encodeDepth++
	i := e.encBuf.begin(tag, TypeStructure)
	err := f(e)
	e.encBuf.end(i)
	e.encodeDepth--

	return err
}

// EncodeEnumeration, along with the other Encode<Type> methods, encodes a
// single KMIP value with the given tag to an internal buffer.  These methods
// do not flush the data to the writer: call Flush() to flush the buffer.
func (e *Encoder) EncodeEnumeration(tag Tag, v uint32) {
	e.encBuf.encodeEnum(tag, v)
}

func (e *Encoder) EncodeInteger(tag Tag, v int32) {
	e.encBuf.encodeInt(tag, v)
}

func (e *Encoder) EncodeLongInteger(tag Tag, v int64) {
	e.encBuf.encodeLongInt(tag, v)
}

func (e *Encoder) EncodeInterval(tag Tag, v time.Duration) {
	e.encBuf.encodeInterval(tag, v)
}

func (e *Encoder) EncodeDateTime(tag Tag, v time.Time) {
	e.encBuf.encodeDateTime(tag, v)
}

func (e *Encoder) EncodeDateTimeExtended(tag Tag, v time.Time) {
	e.encBuf.encodeDateTimeExtended(tag, v)
}

func (e *Encoder) EncodeBigInteger(tag Tag, v *big.Int) {
	e.encBuf.encodeBigInt(tag, v)
}

func (e *Encoder) EncodeBoolean(tag Tag, v bool) {
	e.encBuf.encodeBool(tag, v)
}

func (e *Encoder) EncodeTextString(tag Tag, v string) {
	e.encBuf.encodeTextString(tag, v)
}

func (e *Encoder) EncodeByteString(tag Tag, v []byte) {
	e.encBuf.encodeByteString(tag, v)
}

// Flush flushes the internal encoding buffer to the writer.
func (e *Encoder) Flush() error {
	if e.encodeDepth > 0 {
		return nil
	}

	_, err := e.encBuf.WriteTo(e.w)
	e.encBuf.Reset()

	return err
}

type MarshalerError struct {
	// Type is the golang type of the value being marshaled
	Type reflect.Type
	// Struct is the name of the enclosing struct if the marshaled value is a field.
	Struct string
	// Field is the name of the field being marshaled
	Field string
	Tag   Tag
}

func (e *MarshalerError) Error() string {
	msg := "kmip: error marshaling value"
	if e.Type != nil {
		msg += " of type " + e.Type.String()
	}

	if e.Struct != "" {
		msg += " in struct field " + e.Struct + "." + e.Field
	}

	return msg
}

func (e *Encoder) marshalingError(tag Tag, t reflect.Type, cause error) merry.Error {
	err := &MarshalerError{
		Type:   t,
		Struct: e.currStruct,
		Field:  e.currField,
		Tag:    tag,
	}

	return merry.WrapSkipping(err, 1).WithCause(cause)
}

var (
	byteType        = reflect.TypeOf(byte(0))
	marshalerType   = reflect.TypeOf((*Marshaler)(nil)).Elem()
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	timeType        = reflect.TypeOf((*time.Time)(nil)).Elem()
	bigIntPtrType   = reflect.TypeOf((*big.Int)(nil))
	bigIntType      = bigIntPtrType.Elem()
	durationType    = reflect.TypeOf(time.Nanosecond)
	ttlvType        = reflect.TypeOf((*TTLV)(nil)).Elem()
	tagType         = reflect.TypeOf(Tag(0))
)

var invalidValue = reflect.Value{}

// indirect dives into interfaces values, and one level deep into pointers
// returns an invalid value if the resolved value is nil or invalid.
func indirect(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}

	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}

	if !v.IsValid() {
		return v
	}

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Func, reflect.Slice, reflect.Map, reflect.Chan, reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return invalidValue
		}
	default:
	}

	return v
}

var zeroBigInt = big.Int{}

func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	default:
	}

	switch v.Type() {
	case timeType:
		return v.Interface().(time.Time).IsZero() //nolint:forcetypeassert
	case bigIntType:
		i := v.Interface().(big.Int) //nolint:forcetypeassert
		return zeroBigInt.Cmp(&i) == 0
	}

	return false
}

func (e *Encoder) encode(tag Tag, v reflect.Value, fi *fieldInfo) error {
	// if pointer or interface
	v = indirect(v)
	if !v.IsValid() {
		return nil
	}

	typ := v.Type()

	if typ == ttlvType {
		// fast path: if the value is TTLV, we write it directly to the output buffer
		_, err := e.encBuf.Write(v.Bytes())
		return err
	}

	typeInfo, err := getTypeInfo(typ)
	if err != nil {
		return err
	}

	if tag == TagNone {
		tag = tagForMarshal(v, typeInfo, fi)
	}

	var flags fieldFlags
	if fi != nil {
		flags = fi.flags
	}

	// check for Marshaler
	switch {
	case typ.Implements(marshalerType):
		if flags.omitEmpty() && isEmptyValue(v) {
			return nil
		}

		return v.Interface().(Marshaler).MarshalTTLV(e, tag) //nolint:forcetypeassert
	case v.CanAddr():
		pv := v.Addr()

		pvtyp := pv.Type()
		if pvtyp.Implements(marshalerType) {
			if flags.omitEmpty() && isEmptyValue(v) {
				return nil
			}

			return pv.Interface().(Marshaler).MarshalTTLV(e, tag) //nolint:forcetypeassert
		}
	}

	// If the type doesn't implement Marshaler, then validate the value is a supported kind
	switch v.Kind() {
	case reflect.Chan, reflect.Map, reflect.Func, reflect.Ptr, reflect.UnsafePointer, reflect.Uintptr, reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Interface:
		return e.marshalingError(tag, v.Type(), ErrUnsupportedTypeError)
	default:
	}

	// skip if value is empty and tags include omitempty
	if flags.omitEmpty() && isEmptyValue(v) {
		return nil
	}

	// recurse to handle slices of values
	switch v.Kind() {
	case reflect.Slice:
		if typ.Elem() == byteType {
			// special case, encode as a ByteString, handled below
			break
		}

		fallthrough
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			// turn off the omit empty flag.  applies at the field level,
			// not to each member of the slice
			// TODO: is this true?
			var fi2 *fieldInfo
			if fi != nil {
				fi2 = &fieldInfo{}
				// make a copy.
				*fi2 = *fi
				fi2.flags &^= fOmitEmpty
			}

			err := e.encode(tag, v.Index(i), fi2)
			if err != nil {
				return err
			}
		}

		return nil
	default:
	}

	if tag == TagNone {
		return e.marshalingError(tag, v.Type(), ErrNoTag)
	}

	// handle enums and bitmasks
	//
	// If the field has the "enum" or "bitmask" flag, or the tag is registered as an enum or bitmask,
	// attempt to interpret the go value as such.
	//
	// If the field is explicitly flag, return an error if the value can't be interpreted.  Otherwise
	// ignore errors and let processing fallthrough to the type-based encoding.
	enumMap := DefaultRegistry.EnumForTag(tag)
	if flags.enum() || flags.bitmask() || enumMap != nil {
		switch typ.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
			i := v.Int()

			if flags.bitmask() || (enumMap != nil && enumMap.Bitmask()) {
				e.encBuf.encodeInt(tag, int32(i)) //nolint:gosec // already prevented by the check above
			} else {
				e.encBuf.encodeEnum(tag, uint32(i)) //nolint:gosec // already prevented by the check above
			}

			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
			i := v.Uint()

			if flags.bitmask() || (enumMap != nil && enumMap.Bitmask()) {
				e.encBuf.encodeInt(tag, int32(i)) //nolint:gosec // already prevented by the check above
			} else {
				e.encBuf.encodeEnum(tag, uint32(i)) //nolint:gosec // already prevented by the check above
			}

			return nil
		case reflect.String:
			s := v.String()

			if flags.bitmask() || (enumMap != nil && enumMap.Bitmask()) {
				i, err := ParseInt(s, enumMap)
				if err == nil {
					e.encBuf.encodeInt(tag, i)
					return nil
				}
				// only throw an error if the field is explicitly marked as a bitmask
				// otherwise just ignore it, and let it encode as a string later on.
				if flags.bitmask() {
					// if we couldn't parse the string as an enum value
					return e.marshalingError(tag, typ, err)
				}
			} else {
				i, err := ParseEnum(s, enumMap)
				if err == nil {
					e.encBuf.encodeEnum(tag, i)
					return nil
				}
				// only throw an error if the field is explicitly marked as an enum
				// otherwise just ignore it, and let it encode as a string later on.
				if flags.enum() {
					// if we couldn't parse the string as an enum value
					return e.marshalingError(tag, typ, err)
				}
			}
		default:
			if flags.enum() || flags.bitmask() {
				return e.marshalingError(tag, typ, ErrUnsupportedEnumTypeError).Append(typ.String())
			}
		}
	}

	// handle special types
	switch typ {
	case timeType:
		if flags.dateTimeExt() {
			e.encBuf.encodeDateTimeExtended(tag, v.Interface().(time.Time)) //nolint:forcetypeassert
		} else {
			e.encBuf.encodeDateTime(tag, v.Interface().(time.Time)) //nolint:forcetypeassert
		}

		return nil
	case bigIntType:
		bi := v.Interface().(big.Int) //nolint:forcetypeassert
		e.encBuf.encodeBigInt(tag, &bi)

		return nil
	case bigIntPtrType:
		e.encBuf.encodeBigInt(tag, v.Interface().(*big.Int)) //nolint:forcetypeassert
		return nil
	case durationType:
		e.encBuf.encodeInterval(tag, time.Duration(v.Int()))
		return nil
	}

	// handle the rest of the kinds
	switch typ.Kind() {
	case reflect.Struct:
		// push current struct onto stack
		currStruct := e.currStruct
		e.currStruct = typ.Name()

		err = e.EncodeStructure(tag, func(e *Encoder) error {
			for _, field := range typeInfo.valueFields {
				fv := v.FieldByIndex(field.index)

				// note: we're staying in reflection world here instead of
				// converting back to an interface{} value and going through
				// the non-reflection path again.  Calling Interface()
				// on the reflect value would make a potentially addressable value
				// into an unaddressable value, reducing the chances we can coerce
				// the value into a Marshalable.
				//
				// tl;dr
				// Consider a type which implements Marshaler with
				// a pointer receiver, and a struct with a non-pointer field of that type:
				//
				//     type Wheel struct{}
				//     func (*Wheel) MarshalTTLV(...)
				//
				//     type Car struct{
				//         Wheel Wheel
				//     }
				//
				// When traversing the Car struct, should the encoder invoke Wheel's
				// Marshaler method, or not?  Technically, the type `Wheel`
				// doesn't implement the Marshaler interface.  Only the type `*Wheel`
				// implements it.  However, the other encoders in the SDK, like JSON
				// and XML, will try, if possible, to get a pointer to field values like this, in
				// order to invoke the Marshaler interface anyway.
				//
				// Encoders can only get a pointer to field values if the field
				// value is `addressable`.  Addressability is explained in the docs for reflect.Value#CanAddr().
				// Using reflection to turn a reflect.Value() back into an interface{}
				// can make a potentially addressable value (like the field of an addressable struct)
				// into an unaddressable value (reflect.Value#Interface{} always returns an unaddressable
				// copy).

				// push the currField
				currField := e.currField
				e.currField = field.name
				err := e.encode(TagNone, fv, &field)
				// pop the currField
				e.currField = currField
				if err != nil {
					return err
				}
			}

			return nil
		})
		// pop current struct
		e.currStruct = currStruct

		return err
	case reflect.String:
		e.encBuf.encodeTextString(tag, v.String())
	case reflect.Slice:
		// special case, encode as a ByteString
		// all slices which aren't []byte should have been handled above
		// the call to v.Bytes() will panic if this assumption is wrong
		e.encBuf.encodeByteString(tag, v.Bytes())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		i := v.Int()
		if i > math.MaxInt32 {
			return e.marshalingError(tag, typ, ErrIntOverflow)
		}

		e.encBuf.encodeInt(tag, int32(i)) //nolint:gosec // already prevented by the check above

		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		u := v.Uint()
		if u > math.MaxInt32 {
			return e.marshalingError(tag, typ, ErrIntOverflow)
		}

		e.encBuf.encodeInt(tag, int32(u))

		return nil
	case reflect.Uint64:
		u := v.Uint()
		e.encBuf.encodeLongInt(tag, int64(u)) //nolint:gosec // even though its cast to int, the value will be encoded the same as a uint

		return nil
	case reflect.Int64:
		e.encBuf.encodeLongInt(tag, v.Int())
		return nil
	case reflect.Bool:
		e.encBuf.encodeBool(tag, v.Bool())
	default:
		// all kinds should have been handled by now
		panic(errors.New("should never get here"))
	}

	return nil
}

func tagForMarshal(v reflect.Value, ti typeInfo, fi *fieldInfo) Tag {
	// the tag on the TTLVTag field
	if ti.tagField != nil && ti.tagField.explicitTag != TagNone {
		return ti.tagField.explicitTag
	}

	// the value of the TTLVTag field of type Tag
	if v.IsValid() && ti.tagField != nil && ti.tagField.ti.typ == tagType {
		tag := v.FieldByIndex(ti.tagField.index).Interface().(Tag) //nolint:forcetypeassert
		if tag != TagNone {
			return tag
		}
	}

	// if value is in a struct field, infer the tag from the field
	// else infer from the value's type name
	if fi != nil {
		return fi.tag
	}

	return ti.inferredTag
}

// encBuf encodes basic KMIP types into TTLV.
type encBuf struct {
	bytes.Buffer
}

func (h *encBuf) begin(tag Tag, typ Type) int {
	_ = h.WriteByte(byte(tag >> 16))
	_ = h.WriteByte(byte(tag >> 8))
	_ = h.WriteByte(byte(tag))
	_ = h.WriteByte(byte(typ))
	_, _ = h.Write(zeros[:4])

	return h.Len()
}

func (h *encBuf) end(i int) {
	n := h.Len() - i
	if m := n % 8; m > 0 {
		_, _ = h.Write(zeros[:8-m])
	}

	binary.BigEndian.PutUint32(h.Bytes()[i-4:], uint32(n)) //nolint:gosec
}

func (h *encBuf) writeLongIntVal(tag Tag, typ Type, i int64) {
	s := h.begin(tag, typ)
	ll := h.Len()
	_, _ = h.Write(zeros[:8])
	binary.BigEndian.PutUint64(h.Bytes()[ll:], uint64(i)) //nolint:gosec
	h.end(s)
}

func (h *encBuf) writeIntVal(tag Tag, typ Type, val uint32) {
	s := h.begin(tag, typ)
	ll := h.Len()
	_, _ = h.Write(zeros[:4])
	binary.BigEndian.PutUint32(h.Bytes()[ll:], val)
	h.end(s)
}

var (
	ones  = [8]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	zeros = [8]byte{}
)

func (h *encBuf) encodeBigInt(tag Tag, i *big.Int) {
	if i == nil {
		return
	}

	ii := h.begin(tag, TypeBigInteger)

	switch i.Sign() {
	case 0:
		_, _ = h.Write(zeros[:8])
	case 1:
		b := i.Bytes()
		l := len(b)
		// if n is positive, but the first bit is a 1, it will look like
		// a negative in 2's complement, so prepend zeroes in front
		if b[0]&0x80 > 0 {
			_ = h.WriteByte(byte(0))
			l++
		}
		// pad front with zeros to multiple of 8
		if m := l % 8; m > 0 {
			_, _ = h.Write(zeros[:8-m])
		}

		_, _ = h.Write(b)
	case -1:
		length := uint(i.BitLen()/8+1) * 8 //nolint:gosec
		j := new(big.Int).Lsh(one, length)
		b := j.Add(i, j).Bytes()
		// When the most significant bit is on a byte
		// boundary, we can get some extra significant
		// bits, so strip them off when that happens.
		if len(b) >= 2 && b[0] == 0xff && b[1]&0x80 != 0 {
			b = b[1:]
		}

		l := len(b)
		// pad front with ones to multiple of 8
		if m := l % 8; m > 0 {
			_, _ = h.Write(ones[:8-m])
		}

		_, _ = h.Write(b)
	}

	h.end(ii)
}

func (h *encBuf) encodeInt(tag Tag, i int32) {
	h.writeIntVal(tag, TypeInteger, uint32(i)) //nolint:gosec
}

func (h *encBuf) encodeBool(tag Tag, b bool) {
	if b {
		h.writeLongIntVal(tag, TypeBoolean, 1)
	} else {
		h.writeLongIntVal(tag, TypeBoolean, 0)
	}
}

func (h *encBuf) encodeLongInt(tag Tag, i int64) {
	h.writeLongIntVal(tag, TypeLongInteger, i)
}

func (h *encBuf) encodeDateTime(tag Tag, t time.Time) {
	h.writeLongIntVal(tag, TypeDateTime, t.Unix())
}

func (h *encBuf) encodeDateTimeExtended(tag Tag, t time.Time) {
	// take unix seconds, times a million, to get microseconds, then
	// add nanoseconds remainder/1000
	//
	// this gives us a larger ranger of possible values than just t.UnixNano() / 1000.
	// see UnixNano() docs for its limits.
	//
	// this is limited to max(int64) *microseconds* from epoch, rather than
	// max(int64) nanoseconds like UnixNano().
	m := (t.Unix() * 1000000) + int64(t.Nanosecond()/1000)
	h.writeLongIntVal(tag, TypeDateTimeExtended, m)
}

func (h *encBuf) encodeInterval(tag Tag, d time.Duration) {
	h.writeIntVal(tag, TypeInterval, uint32(d/time.Second)) //nolint:gosec
}

func (h *encBuf) encodeEnum(tag Tag, i uint32) {
	h.writeIntVal(tag, TypeEnumeration, i)
}

func (h *encBuf) encodeTextString(tag Tag, s string) {
	i := h.begin(tag, TypeTextString)
	_, _ = h.WriteString(s)
	h.end(i)
}

func (h *encBuf) encodeByteString(tag Tag, b []byte) {
	if b == nil {
		return
	}

	i := h.begin(tag, TypeByteString)
	_, _ = h.Write(b)
	h.end(i)
}

func getTypeInfo(typ reflect.Type) (ti typeInfo, err error) {
	ti.inferredTag, _ = DefaultRegistry.ParseTag(typ.Name())
	ti.typ = typ
	err = ti.getFieldsInfo()

	return ti, err
}

var errSkip = errors.New("skip")

func getFieldInfo(typ reflect.Type, sf reflect.StructField) (fieldInfo, error) {
	var fi fieldInfo

	// skip anonymous and unexported fields
	if sf.Anonymous || /*unexported:*/ sf.PkgPath != "" {
		return fi, errSkip
	}

	fi.name = sf.Name
	fi.structType = typ
	fi.index = sf.Index

	var anyField bool

	// handle field tags
	parts := strings.Split(sf.Tag.Get(structFieldTag), ",")
	for i, value := range parts {
		if i == 0 {
			switch value {
			case "-":
				// skip
				return fi, errSkip
			case "":
			default:
				var err error

				fi.explicitTag, err = DefaultRegistry.ParseTag(value)
				if err != nil {
					return fi, err
				}
			}
		} else {
			switch strings.ToLower(value) {
			case "enum":
				fi.flags |= fEnum
			case "omitempty":
				fi.flags |= fOmitEmpty
			case "datetimeextended":
				fi.flags |= fDateTimeExtended
			case "bitmask":
				fi.flags |= fBitBask
			case "any":
				anyField = true
				fi.flags |= fAny
			}
		}
	}

	if anyField && fi.explicitTag != TagNone {
		return fi, merry.Here(ErrTagConflict).Appendf(`field %s.%s may not specify a TTLV tag and the "any" flag`, fi.structType.Name(), fi.name)
	}

	// extract type info for the field.  The KMIP tag
	// for this field is derived from either the field name,
	// the field tags, or the field type.
	var err error

	fi.ti, err = getTypeInfo(sf.Type)
	if err != nil {
		return fi, err
	}

	if fi.ti.tagField != nil && fi.ti.tagField.explicitTag != TagNone {
		fi.tag = fi.ti.tagField.explicitTag
		if fi.explicitTag != TagNone && fi.explicitTag != fi.tag {
			// if there was a tag on the struct field containing this value, it must
			// agree with the value's intrinsic tag
			return fi, merry.Here(ErrTagConflict).Appendf(`TTLV tag "%s" in tag of %s.%s conflicts with TTLV tag "%s" in %s.%s`, fi.explicitTag, fi.structType.Name(), fi.name, fi.ti.tagField.explicitTag, fi.ti.typ.Name(), fi.ti.tagField.name)
		}
	}

	// pre-calculate the tag for this field.  This intentional duplicates
	// some of tagForMarshaling().  The value is primarily used in unmarshaling
	// where the dynamic value of the field is not needed.
	if fi.tag == TagNone {
		fi.tag = fi.explicitTag
	}

	if fi.tag == TagNone {
		fi.tag, _ = DefaultRegistry.ParseTag(fi.name)
	}

	return fi, nil
}

func (ti *typeInfo) getFieldsInfo() error {
	if ti.typ.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < ti.typ.NumField(); i++ {
		fi, err := getFieldInfo(ti.typ, ti.typ.Field(i))

		switch {
		case err == errSkip: //nolint:errorlint
			// skip
		case err != nil:
			return err
		case fi.name == "TTLVTag":
			ti.tagField = &fi
		default:
			ti.valueFields = append(ti.valueFields, fi)
		}
	}

	// verify that multiple fields don't have the same tag
	names := map[Tag]string{}

	for _, f := range ti.valueFields {
		if f.flags.any() {
			// ignore any fields
			continue
		}

		tag := f.tag
		if tag != TagNone {
			if fname, ok := names[tag]; ok {
				return merry.Here(ErrTagConflict).Appendf("field resolves to the same tag (%s) as other field (%s)", tag, fname)
			}

			names[tag] = f.name
		}
	}

	return nil
}

type typeInfo struct {
	typ         reflect.Type
	inferredTag Tag
	tagField    *fieldInfo
	valueFields []fieldInfo
}

const (
	fOmitEmpty fieldFlags = 1 << iota
	fEnum
	fDateTimeExtended
	fAny
	fBitBask
)

type fieldFlags int

func (f fieldFlags) omitEmpty() bool {
	return f&fOmitEmpty != 0
}

func (f fieldFlags) any() bool {
	return f&fAny != 0
}

func (f fieldFlags) dateTimeExt() bool {
	return f&fDateTimeExtended != 0
}

func (f fieldFlags) enum() bool {
	return f&fEnum != 0
}

func (f fieldFlags) bitmask() bool {
	return f&fBitBask != 0
}

type fieldInfo struct {
	structType       reflect.Type
	explicitTag, tag Tag
	name             string
	index            []int
	flags            fieldFlags
	ti               typeInfo
}
