package flume

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var _ Logger = (*Core)(nil)

type atomicInnerCore struct {
	innerLoggerPtr atomic.Value
}

func (af *atomicInnerCore) get() *innerCore {
	return af.innerLoggerPtr.Load().(*innerCore)
}

func (af *atomicInnerCore) set(ic *innerCore) {
	af.innerLoggerPtr.Store(ic)
}

// innerCore holds state which can be reconfigured at the factory level.
// if these settings are changed in the factory, the factory builds new
// innerCore instances, and atomically injects them into all existing loggers.
type innerCore struct {
	name string
	zapcore.Core
	addCaller   bool
	errorOutput zapcore.WriteSyncer
	hooks       []HookFunc
}

// Core is the concrete implementation of Logger.  It has some additional
// lower-level methods which can be used by other logging packages which wrap
// flume, to build alternate logging interfaces.
type Core struct {
	*atomicInnerCore
	context    []zap.Field
	callerSkip int
	// these are logger-scoped hooks, which only hook into this particular logger
	hooks []HookFunc
}

// Log is the core logging method, used by the convenience methods Debug(), Info(), and Error().
//
// Returns true if the log was actually logged.
//
// AddCaller option will report the caller of this method.  If wrapping this, be sure to
// use the AddCallerSkip option.
func (l *Core) Log(lvl Level, template string, fmtArgs, context []interface{}) bool {
	// call another method, just to add a caller to the call stack, so the
	// add caller option resolves the right caller in the stack
	return l.log(lvl, template, fmtArgs, context)
}

// log must be called directly from one of the public methods to make the addcaller
// resolution resolve the caller of the public method.
func (l *Core) log(lvl Level, template string, fmtArgs, context []interface{}) bool {
	c := l.get()

	if !c.Enabled(zapcore.Level(lvl)) {
		return false
	}

	msg := template
	if msg == "" && len(fmtArgs) > 0 {
		msg = fmt.Sprint(fmtArgs...)
	} else if msg != "" && len(fmtArgs) > 0 {
		msg = fmt.Sprintf(template, fmtArgs...)
	}

	// check must always be called directly by a method in the Logger interface
	// (e.g., Log, Info, Debug).
	const callerSkipOffset = 2

	// Create basic checked entry thru the core; this will be non-nil if the
	// log message will actually be written somewhere.
	ent := zapcore.Entry{
		LoggerName: c.name,
		Time:       time.Now(),
		Level:      zapcore.Level(lvl),
		Message:    msg,
	}
	ce := c.Check(ent, nil)
	if ce == nil {
		return false
	}

	// Thread the error output through to the CheckedEntry.
	ce.ErrorOutput = c.errorOutput
	if c.addCaller {
		ce.Entry.Caller = zapcore.NewEntryCaller(runtime.Caller(l.callerSkip + callerSkipOffset))
		if !ce.Entry.Caller.Defined {
			_, _ = fmt.Fprintf(c.errorOutput, "%v Logger.check error: failed to get caller\n", time.Now().UTC())
			_ = ce.ErrorOutput.Sync()
		}
	}

	fields := append(l.context, l.sweetenFields(context)...) //nolint:gocritic

	// execute global hooks, which might modify the fields
	for i := range c.hooks {
		if f := c.hooks[i](ce, fields); f != nil {
			fields = f
		}
	}

	// execute logger hooks
	for i := range l.hooks {
		if f := l.hooks[i](ce, fields); f != nil {
			fields = f
		}
	}

	ce.Write(fields...)
	return true
}

// IsEnabled returns true if the specified level is enabled.
func (l *Core) IsEnabled(lvl Level) bool {
	return l.get().Enabled(zapcore.Level(lvl))
}

const (
	_oddNumberErrMsg    = "Ignored key without a value."
	_nonStringKeyErrMsg = "Ignored key-value pairs with non-string keys."
)

func (l *Core) sweetenFields(args []interface{}) []zap.Field {
	if len(args) == 0 {
		return nil
	}

	// Allocate enough space for the worst case; if users pass only structured
	// fields, we shouldn't penalize them with extra allocations.
	fields := make([]zap.Field, 0, len(args))
	var invalid invalidPairs

	for i := 0; i < len(args); {
		// This is a strongly-typed field. Consume it and move on.
		if f, ok := args[i].(zap.Field); ok {
			fields = append(fields, f)
			i++
			continue
		}

		if len(args) == 1 {
			// passed a bare arg with no key.  We'll handle this
			// as a special case
			if err, ok := args[0].(error); ok {
				return append(fields, zap.Error(err))
			}
			return append(fields, zap.Any("", args[0]))
		}

		// Make sure this element isn't a dangling key.
		if i == len(args)-1 {
			l.Error(_oddNumberErrMsg, zap.Any("ignored", args[i]))
			break
		}

		// Consume this value and the next, treating them as a key-value pair. If the
		// key isn't a string, add this pair to the slice of invalid pairs.
		key, val := args[i], args[i+1]
		if keyStr, ok := key.(string); !ok {
			// Subsequent errors are likely, so allocate once up front.
			if cap(invalid) == 0 {
				invalid = make(invalidPairs, 0, len(args)/2)
			}
			invalid = append(invalid, invalidPair{i, key, val})
		} else {
			fields = append(fields, zap.Any(keyStr, val))
		}
		i += 2
	}

	// If we encountered any invalid key-value pairs, log an error.
	if len(invalid) > 0 {
		l.Error(_nonStringKeyErrMsg, zap.Array("invalid", invalid))
	}
	return fields
}

type invalidPair struct {
	position   int
	key, value interface{}
}

func (p invalidPair) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddInt64("position", int64(p.position))
	zap.Any("key", p.key).AddTo(enc)
	zap.Any("value", p.value).AddTo(enc)
	return nil
}

type invalidPairs []invalidPair

func (ps invalidPairs) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	var err error
	for i := range ps {
		err = multierr.Append(err, enc.AppendObject(ps[i]))
	}
	return err
}

// Debug logs at DBG level.  args should be alternative keys and values.  keys should be strings.
func (l *Core) Debug(msg string, args ...interface{}) {
	l.log(DebugLevel, msg, nil, args)
}

// Info logs at INF level. args should be alternative keys and values.  keys should be strings.
func (l *Core) Info(msg string, args ...interface{}) {
	l.log(InfoLevel, msg, nil, args)
}

// Error logs at ERR level.  args should be alternative keys and values.  keys should be strings.
func (l *Core) Error(msg string, args ...interface{}) {
	l.log(ErrorLevel, msg, nil, args)
}

// IsDebug returns true if DBG level is enabled.
func (l *Core) IsDebug() bool {
	return l.IsEnabled(DebugLevel)
}

// IsDebug returns true if INF level is enabled
func (l *Core) IsInfo() bool {
	return l.IsEnabled(InfoLevel)
}

// With returns a new Logger with some context baked in.  All entries
// logged with the new logger will include this context.
//
// args should be alternative keys and values.  keys should be strings.
//
//	reqLogger := l.With("requestID", reqID)
func (l *Core) With(args ...interface{}) Logger {
	return l.WithArgs(args...)
}

// WithArgs is the same as With() but returns the concrete type.  Useful
// for other logging packages which wrap this one.
func (l *Core) WithArgs(args ...interface{}) *Core {
	l2 := l.clone()
	switch len(args) {
	case 0:
	default:
		l2.context = append(l2.context, l.sweetenFields(args)...)
	}
	return l2
}

func (l *Core) clone() *Core {
	l2 := *l
	l2.context = nil
	if len(l.context) > 0 {
		l2.context = append(l2.context, l.context...)
	}
	return &l2
}

// ZapCore returns a zapcore.Core that can be used to write logs to a zap logger, or to
// integrate with other logging systems.
func (l *Core) ZapCore() zapcore.Core {
	return &zapCore{c: l}
}

type zapCore struct {
	c *Core
}

var _ zapcore.Core = (*zapCore)(nil)

func (l *zapCore) Enabled(lvl zapcore.Level) bool {
	return l.c.get().Enabled(lvl)
}

func (l *zapCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	// DO NOT pass this on to the inner core, because the inner core will
	// add itself to the CheckedEntry, so when CheckEntry is called, it will
	// it will only invoke Write on the inner core, not the outer core.  Our outer
	// core will not have the chance to add the context fields to the CheckedEntry.
	c := l.c.get()
	if c.Enabled(entry.Level) {
		ce = ce.AddCore(entry, l)
		ce.ErrorOutput = c.errorOutput
		return ce
	}
	return nil
}

func (l *zapCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	return l.c.get().Write(entry, append(l.c.context, fields...))
}

func (l *zapCore) Sync() error {
	return l.c.get().Sync()
}

func (l *zapCore) With(fields []zapcore.Field) zapcore.Core {
	if len(fields) == 0 {
		return l
	}
	l2 := l.c.clone()
	l2.context = append(l2.context, fields...)
	return &zapCore{c: l2}
}
