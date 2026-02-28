package flume

import (
	"encoding/hex"
	"github.com/mgutz/ansi"
	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

//nolint:gochecknoinits
func init() {
	_ = zap.RegisterEncoder("term", func(cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
		return NewConsoleEncoder((*EncoderConfig)(&cfg)), nil
	})
	_ = zap.RegisterEncoder("term-color", func(cfg zapcore.EncoderConfig) (zapcore.Encoder, error) {
		return NewColorizedConsoleEncoder((*EncoderConfig)(&cfg), nil), nil
	})
}

// Colorizer returns ansi escape sequences for the colors for each log level.
// See Colors for a default implementation.
type Colorizer interface {
	Level(l Level) string
}

// Colors is an implementation of the Colorizer interface, which assigns colors
// to the default log levels.
type Colors struct {
	Debug, Info, Warn, Error string
}

// Level implements Colorizer
func (c *Colors) Level(l Level) string {
	if l < DebugLevel {
		return Dim
	}
	switch l {
	case DebugLevel:
		return c.Debug
	case InfoLevel:
		return c.Info
	case Level(zapcore.WarnLevel):
		return c.Warn
	default:
		return c.Error
	}
}

// DefaultColors is the default instance of Colors, used as the default colors if
// a nil Colorizer is passed to NewColorizedConsoleEncoder.
var DefaultColors = Colors{
	Debug: ansi.ColorCode("cyan"),
	Info:  ansi.ColorCode("green+h"),
	Warn:  ansi.ColorCode("yellow+bh"),
	Error: ansi.ColorCode("red+bh"),
}

type consoleEncoder struct {
	*ltsvEncoder
	colorizer Colorizer
}

// NewConsoleEncoder creates an encoder whose output is designed for human -
// rather than machine - consumption. It serializes the core log entry data
// (message, level, timestamp, etc.) in a plain-text format.  The context is
// encoded in LTSV.
//
// Note that although the console encoder doesn't use the keys specified in the
// encoder configuration, it will omit any element whose key is set to the empty
// string.
func NewConsoleEncoder(cfg *EncoderConfig) Encoder {
	ltsvEncoder := NewLTSVEncoder(cfg).(*ltsvEncoder)
	ltsvEncoder.allowNewLines = true
	ltsvEncoder.allowTabs = true
	ltsvEncoder.blankKey = "value"
	ltsvEncoder.binaryEncoder = hex.Dump

	return &consoleEncoder{ltsvEncoder: ltsvEncoder}
}

// NewColorizedConsoleEncoder creates a console encoder, like NewConsoleEncoder, but
// colors the text with ansi escape codes.  `colorize` configures which colors to
// use for each level.
//
// If `colorizer` is nil, it will default to DefaultColors.
//
// `github.com/mgutz/ansi` is a convenient package for getting color codes, e.g.:
//
//	ansi.ColorCode("red")
func NewColorizedConsoleEncoder(cfg *EncoderConfig, colorizer Colorizer) Encoder {
	e := NewConsoleEncoder(cfg).(*consoleEncoder)
	e.colorizer = colorizer
	if e.colorizer == nil {
		e.colorizer = &DefaultColors
	}
	return e
}

// Clone implements the Encoder interface
func (c *consoleEncoder) Clone() zapcore.Encoder {
	return &consoleEncoder{
		ltsvEncoder: c.ltsvEncoder.Clone().(*ltsvEncoder),
		colorizer:   c.colorizer,
	}
}

// Dim is the color used for context keys, time, and caller information
var Dim = ansi.ColorCode("240")

// Bright is the color used for the message
var Bright = ansi.ColorCode("default+b")

// EncodeEntry implements the Encoder interface
func (c *consoleEncoder) EncodeEntry(ent zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	final := *c.ltsvEncoder
	context := final.buf
	final.buf = bufPool.Get()

	origLen := final.buf.Len()

	if c.TimeKey != "" {
		c.colorDim(final.buf)
		final.skipNextElementSeparator = true
		c.EncodeTime(ent.Time, &final)
	}

	if c.LevelKey != "" {
		c.colorLevel(final.buf, ent.Level)
		if final.buf.Len() > origLen {
			final.buf.AppendByte(' ')
		}
		final.skipNextElementSeparator = true

		c.EncodeLevel(ent.Level, &final)

	}

	if final.buf.Len() > origLen {
		c.colorDim(final.buf)
		final.buf.AppendString(" | ")
	} else {
		final.buf.Reset()
	}

	// Add the message itself.
	if c.MessageKey != "" {
		c.colorReset(final.buf)
		// c.colorBright(&final)
		final.safeAddString(ent.Message, false)
		// ensure a minimum of 2 spaces between the message and the fields,
		// to improve readability
		final.buf.AppendString("  ")
	}

	c.colorDim(final.buf)

	// Add fields.
	for _, f := range fields {
		f.AddTo(&final)
	}

	// Add context
	if context.Len() > 0 {
		final.addFieldSeparator()
		_, _ = final.buf.Write(context.Bytes())
	}

	// Add callsite
	c.writeCallSite(&final, ent.LoggerName, ent.Caller)

	// If there's no stacktrace key, honor that; this allows users to force
	// single-line output.
	if ent.Stack != "" && c.StacktraceKey != "" {
		final.buf.AppendByte('\n')
		final.buf.AppendString(ent.Stack)
	}
	c.colorReset(final.buf)
	final.buf.AppendByte('\n')

	return final.buf, nil
}

func (c *consoleEncoder) writeCallSite(final *ltsvEncoder, name string, caller zapcore.EntryCaller) {
	shouldWriteName := name != "" && c.NameKey != ""
	shouldWriteCaller := caller.Defined && c.CallerKey != ""
	if !shouldWriteName && !shouldWriteCaller {
		return
	}
	final.addKey("@")
	if shouldWriteName {
		final.buf.AppendString(name)
		if shouldWriteCaller {
			final.buf.AppendByte('@')
		}
	}
	if shouldWriteCaller {
		final.skipNextElementSeparator = true
		final.EncodeCaller(caller, final)
	}
}

func (c *consoleEncoder) colorDim(buf *buffer.Buffer) {
	c.applyColor(buf, Dim)
}

func (c *consoleEncoder) colorLevel(buf *buffer.Buffer, level zapcore.Level) {
	if c.colorizer != nil {
		c.applyColor(buf, c.colorizer.Level(Level(level)))
	}
}

func (c *consoleEncoder) applyColor(buf *buffer.Buffer, s string) {
	if c.colorizer != nil {
		buf.AppendString(ansi.Reset)
		if s != "" {
			buf.AppendString(s)
		}
	}
}

func (c *consoleEncoder) colorReset(buf *buffer.Buffer) {
	c.applyColor(buf, "")
}
