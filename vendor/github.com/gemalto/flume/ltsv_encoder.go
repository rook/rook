package flume

// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

import (
	"math"
	"time"
	"unicode/utf8"

	"bytes"
	"encoding/base64"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
	"strings"
)

//nolint:gochecknoinits
func init() {
	_ = zap.RegisterEncoder("ltsv", func(cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
		return NewLTSVEncoder((*EncoderConfig)(&cfg)), nil
	})
}

type ltsvEncoder struct {
	*EncoderConfig
	buf                      *buffer.Buffer
	allowTabs                bool
	allowNewLines            bool
	skipNextElementSeparator bool
	lastElementWasMultiline  bool
	fieldNamePrefix          string
	nestingLevel             int
	blankKey                 string
	binaryEncoder            func([]byte) string
}

// NewLTSVEncoder creates a fast, low-allocation LTSV encoder.
func NewLTSVEncoder(cfg *EncoderConfig) Encoder {
	return &ltsvEncoder{
		EncoderConfig: cfg,
		buf:           bufPool.Get(),
		blankKey:      "_",
		binaryEncoder: base64.StdEncoding.EncodeToString,
	}
}

// AddBinary implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddBinary(key string, value []byte) {
	enc.AddString(key, enc.binaryEncoder(value))
}

// AddArray implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddArray(key string, arr zapcore.ArrayMarshaler) error {
	enc.addKey(key)
	return enc.AppendArray(arr)
}

// AddObject implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddObject(key string, obj zapcore.ObjectMarshaler) error {
	enc.addKey(key)
	return enc.AppendObject(obj)
}

// AddBool implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddBool(key string, val bool) {
	enc.addKey(key)
	enc.AppendBool(val)
}

// AddComplex128 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddComplex128(key string, val complex128) {
	enc.addKey(key)
	enc.AppendComplex128(val)
}

// AddDuration implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddDuration(key string, val time.Duration) {
	enc.addKey(key)
	enc.AppendDuration(val)
}

// AddFloat64 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddFloat64(key string, val float64) {
	enc.addKey(key)
	enc.AppendFloat64(val)
}

// AddInt64 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddInt64(key string, val int64) {
	enc.addKey(key)
	enc.AppendInt64(val)
}

// AddReflected implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddReflected(key string, obj interface{}) error {
	enc.addKey(key)
	return enc.AppendReflected(obj)
}

// OpenNamespace implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) OpenNamespace(key string) {
	switch len(enc.fieldNamePrefix) {
	case 0:
		enc.fieldNamePrefix = key
	default:
		enc.fieldNamePrefix = enc.fieldNamePrefix + "." + key

	}
}

// AddString implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddString(key, val string) {
	enc.addKey(key)
	enc.AppendString(val)
}

// AddByteString implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddByteString(key string, value []byte) {
	enc.addKey(key)
	enc.AppendByteString(value)
}

// AddTime implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddTime(key string, val time.Time) {
	enc.addKey(key)
	enc.AppendTime(val)
}

// AddUint64 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddUint64(key string, val uint64) {
	enc.addKey(key)
	enc.AppendUint64(val)
}

// AppendArray implements zapcore.ArrayEncoder
func (enc *ltsvEncoder) AppendArray(arr zapcore.ArrayMarshaler) error {
	enc.addElementSeparator()
	enc.buf.AppendByte('[')
	enc.skipNextElementSeparator = true
	err := arr.MarshalLogArray(enc)
	enc.buf.AppendByte(']')
	enc.skipNextElementSeparator = false
	return err
}

// AppendObject implements zapcore.ArrayEncoder
func (enc *ltsvEncoder) AppendObject(obj zapcore.ObjectMarshaler) error {
	enc.addElementSeparator()
	enc.nestingLevel++
	enc.skipNextElementSeparator = true
	enc.buf.AppendByte('{')
	err := obj.MarshalLogObject(enc)
	enc.buf.AppendByte('}')
	enc.skipNextElementSeparator = false
	enc.nestingLevel--
	return err
}

// AppendBool implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendBool(val bool) {
	enc.addElementSeparator()
	enc.buf.AppendBool(val)
}

// AppendComplex128 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendComplex128(val complex128) {
	enc.addElementSeparator()
	// Cast to a platform-independent, fixed-size type.
	r, i := real(val), imag(val)
	// Because we're always in a quoted string, we can use strconv without
	// special-casing NaN and +/-Inf.
	enc.buf.AppendFloat(r, 64)
	enc.buf.AppendByte('+')
	enc.buf.AppendFloat(i, 64)
	enc.buf.AppendByte('i')
}

// AppendDuration implements zapcore.ArrayEncoder
func (enc *ltsvEncoder) AppendDuration(val time.Duration) {
	enc.EncodeDuration(val, enc)
}

// AppendInt64 implements zapcore.ArrayEncoder
func (enc *ltsvEncoder) AppendInt64(val int64) {
	enc.addElementSeparator()
	enc.buf.AppendInt(val)
}

// AppendReflected implements zapcore.ArrayEncoder
func (enc *ltsvEncoder) AppendReflected(val interface{}) error {
	enc.AppendString(fmt.Sprintf("%+v", val))
	return nil
}

// AppendString implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendString(val string) {
	enc.addElementSeparator()
	if enc.allowNewLines && strings.Contains(val, "\n") {
		enc.safeAddString("\n", false)
	}
	enc.safeAddString(val, false)
}

// AppendByteString implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendByteString(val []byte) {
	enc.addElementSeparator()

	if enc.allowNewLines && bytes.Contains(val, []byte("\n")) {
		enc.safeAddString("\n", false)
	}
	enc.safeAddByteString(val, false)
	panic("implement me")
}

// AppendTime implements zapcore.ArrayEncoder
func (enc *ltsvEncoder) AppendTime(val time.Time) {
	enc.EncodeTime(val, enc)
}

// AppendUint64 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendUint64(val uint64) {
	enc.addElementSeparator()
	enc.buf.AppendUint(val)
}

// AddComplex64 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddComplex64(k string, v complex64) { enc.AddComplex128(k, complex128(v)) }

// AddFloat32 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddFloat32(k string, v float32) { enc.AddFloat64(k, float64(v)) }

// AddInt implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddInt(k string, v int) { enc.AddInt64(k, int64(v)) }

// AddInt32 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddInt32(k string, v int32) { enc.AddInt64(k, int64(v)) }

// AddInt16 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddInt16(k string, v int16) { enc.AddInt64(k, int64(v)) }

// AddInt8 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddInt8(k string, v int8) { enc.AddInt64(k, int64(v)) }

// AddUint implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddUint(k string, v uint) { enc.AddUint64(k, uint64(v)) }

// AddUint32 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddUint32(k string, v uint32) { enc.AddUint64(k, uint64(v)) }

// AddUint16 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddUint16(k string, v uint16) { enc.AddUint64(k, uint64(v)) }

// AddUint8 implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddUint8(k string, v uint8) { enc.AddUint64(k, uint64(v)) }

// AddUintptr implements zapcore.ObjectEncoder
func (enc *ltsvEncoder) AddUintptr(k string, v uintptr) { enc.AddUint64(k, uint64(v)) }

// AppendComplex64 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendComplex64(v complex64) { enc.AppendComplex128(complex128(v)) }

// AppendFloat64 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendFloat64(v float64) { enc.appendFloat(v, 64) }

// AppendFloat32 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendFloat32(v float32) { enc.appendFloat(float64(v), 32) }

// AppendInt implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendInt(v int) { enc.AppendInt64(int64(v)) }

// AppendInt32 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendInt32(v int32) { enc.AppendInt64(int64(v)) }

// AppendInt16 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendInt16(v int16) { enc.AppendInt64(int64(v)) }

// AppendInt8 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendInt8(v int8) { enc.AppendInt64(int64(v)) }

// AppendUint implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendUint(v uint) { enc.AppendUint64(uint64(v)) }

// AppendUint32 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendUint32(v uint32) { enc.AppendUint64(uint64(v)) }

// AppendUint16 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendUint16(v uint16) { enc.AppendUint64(uint64(v)) }

// AppendUint8 implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendUint8(v uint8) { enc.AppendUint64(uint64(v)) }

// AppendUintptr implements zapcore.PrimitiveArrayEncoder
func (enc *ltsvEncoder) AppendUintptr(v uintptr) { enc.AppendUint64(uint64(v)) }

// Clone implements zapcore.Encoder
func (enc *ltsvEncoder) Clone() zapcore.Encoder {
	clone := *enc
	clone.buf = bufPool.Get()
	_, _ = clone.buf.Write(enc.buf.Bytes())
	return &clone
}

// EncodeEntry implements zapcore.Encoder
func (enc *ltsvEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	final := *enc
	final.buf = bufPool.Get()

	if final.LevelKey != "" {
		final.addKey(final.LevelKey)
		final.EncodeLevel(ent.Level, &final)
	}
	if final.TimeKey != "" {
		final.AddTime(final.TimeKey, ent.Time)
	}
	if final.MessageKey != "" {
		final.addKey(enc.MessageKey)
		final.AppendString(ent.Message)
	}
	if ent.LoggerName != "" && final.NameKey != "" {
		final.addKey(final.NameKey)
		final.AppendString(ent.LoggerName)
	}
	if ent.Caller.Defined && final.CallerKey != "" {
		final.addKey(final.CallerKey)
		final.EncodeCaller(ent.Caller, &final)
	}
	if final.buf.Len() > 0 {
		final.addFieldSeparator()
		_, _ = final.buf.Write(enc.buf.Bytes())
	}
	for i := range fields {
		fields[i].AddTo(&final)
	}
	if ent.Stack != "" && final.StacktraceKey != "" {
		final.AddString(final.StacktraceKey, ent.Stack)
	}
	final.buf.AppendByte('\n')
	return final.buf, nil
}

func (enc *ltsvEncoder) addKey(key string) {
	enc.addFieldSeparator()
	switch {
	case key == "" && enc.blankKey == "":
		return
	case key == "" && enc.blankKey != "":
		key = enc.blankKey
	}
	if len(enc.fieldNamePrefix) > 0 {
		enc.safeAddString(enc.fieldNamePrefix, true)
		enc.buf.AppendByte('.')
	}
	enc.safeAddString(key, true)
	enc.buf.AppendByte(':')
}

func (enc *ltsvEncoder) addFieldSeparator() {
	last := enc.buf.Len() - 1
	if last < 0 {
		enc.skipNextElementSeparator = true
		return
	}
	if enc.nestingLevel > 0 {
		enc.addElementSeparator()
		enc.skipNextElementSeparator = true
		return
	}

	lastByte := enc.buf.Bytes()[last]
	if enc.lastElementWasMultiline {
		if lastByte != '\n' && lastByte != '\r' {
			// make sure the last line terminated with a newline
			enc.buf.AppendByte('\n')
		}
		enc.lastElementWasMultiline = false
	} else if lastByte != '\t' {
		enc.buf.AppendByte('\t')
	}
	enc.skipNextElementSeparator = true
}

func (enc *ltsvEncoder) addElementSeparator() {
	if !enc.skipNextElementSeparator && enc.buf.Len() != 0 {
		enc.buf.AppendByte(',')
	}
	enc.skipNextElementSeparator = false
}

func (enc *ltsvEncoder) appendFloat(val float64, bitSize int) {
	enc.addElementSeparator()
	switch {
	case math.IsNaN(val):
		enc.buf.AppendString(`"NaN"`)
	case math.IsInf(val, 1):
		enc.buf.AppendString(`"+Inf"`)
	case math.IsInf(val, -1):
		enc.buf.AppendString(`"-Inf"`)
	default:
		enc.buf.AppendFloat(val, bitSize)
	}
}

// safeAddString appends a string to the internal buffer.
// If `key`, colons are replaced with underscores, and newlines and tabs are escaped
// If not `key`, only newlines and tabs are escaped, unless configured otherwise
//
//nolint:dupl
func (enc *ltsvEncoder) safeAddString(s string, key bool) {
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			i++
			switch {
			case key && b == ':':
				enc.buf.AppendByte('_')
			case b == '\n':
				if !enc.allowNewLines || key {
					enc.buf.AppendString("\\n")
				} else {
					enc.buf.AppendByte(b)
					enc.lastElementWasMultiline = true
				}
			case b == '\r':
				if !enc.allowNewLines || key {
					enc.buf.AppendString("\\r")
				} else {
					enc.buf.AppendByte(b)
					enc.lastElementWasMultiline = true
				}
			case (!enc.allowTabs || key) && b == '\t':
				enc.buf.AppendString("\\t")
			default:
				enc.buf.AppendByte(b)
			}
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			enc.buf.AppendString(`\ufffd`)
			i++
			continue
		}
		enc.buf.AppendString(s[i : i+size])
		i += size
	}
}

// safeAddByteString is no-alloc equivalent of safeAddString(string(s)) for s []byte.
//
//nolint:dupl
func (enc *ltsvEncoder) safeAddByteString(s []byte, key bool) {
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			i++
			switch {
			case key && b == ':':
				enc.buf.AppendByte('_')
			case b == '\n':
				if !enc.allowNewLines || key {
					enc.buf.AppendString("\\n")
				} else {
					enc.buf.AppendByte(b)
					enc.lastElementWasMultiline = true
				}
			case b == '\r':
				if !enc.allowNewLines || key {
					enc.buf.AppendString("\\r")
				} else {
					enc.buf.AppendByte(b)
					enc.lastElementWasMultiline = true
				}
			case (!enc.allowTabs || key) && b == '\t':
				enc.buf.AppendString("\\t")
			default:
				enc.buf.AppendByte(b)
			}
			continue
		}
		c, size := utf8.DecodeRune(s[i:])
		if c == utf8.RuneError && size == 1 {
			enc.buf.AppendString(`\ufffd`)
			i++
			continue
		}
		_, _ = enc.buf.Write(s[i : i+size])
		i += size
	}
}
