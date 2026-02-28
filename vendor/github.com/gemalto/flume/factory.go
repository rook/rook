package flume

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/ansel1/merry"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerInfo struct {
	levelEnabler    zapcore.LevelEnabler
	atomicInnerCore atomicInnerCore
}

// Factory is a log management core.  It spawns loggers.  The Factory has
// methods for dynamically reconfiguring all the loggers spawned from Factory.
//
// The flume package has mirrors of most of the functions which delegate to a
// default, package-level factory.
type Factory struct {
	defaultLevel zap.AtomicLevel

	encoder zapcore.Encoder
	out     io.Writer

	loggers map[string]*loggerInfo
	sync.Mutex

	addCaller bool

	hooks []HookFunc

	newCoreFn func(name string, encoder zapcore.Encoder, out zapcore.WriteSyncer, levelEnabler zapcore.LevelEnabler) zapcore.Core
}

// Encoder serializes log entries.  Re-exported from zap for now to avoid exporting zap.
type Encoder zapcore.Encoder

// NewFactory returns a factory.  The default level is set to OFF (all logs disabled)
func NewFactory() *Factory {
	f := Factory{
		defaultLevel: zap.NewAtomicLevel(),
		loggers:      map[string]*loggerInfo{},
	}
	f.SetDefaultLevel(OffLevel)

	return &f
}

func (r *Factory) getEncoder() zapcore.Encoder {
	if r.encoder == nil {
		return NewLTSVEncoder(NewEncoderConfig())
	}
	return r.encoder
}

// SetEncoder sets the encoder for all loggers created by (in the past or future) this factory.
func (r *Factory) SetEncoder(e Encoder) {
	r.Lock()
	defer r.Unlock()
	r.encoder = e
	r.refreshLoggers()
}

// SetOut sets the output writer for all logs produced by this factory.
// Returns a function which sets the output writer back to the prior setting.
func (r *Factory) SetOut(w io.Writer) func() {
	r.Lock()
	defer r.Unlock()
	prior := r.out
	r.out = w
	r.refreshLoggers()
	return func() {
		r.SetOut(prior)
	}
}

// SetAddCaller enables adding the logging callsite (file and line number) to the log entries.
func (r *Factory) SetAddCaller(b bool) {
	r.Lock()
	defer r.Unlock()
	r.addCaller = b
	r.refreshLoggers()
}

func (r *Factory) AddCaller() bool {
	r.Lock()
	defer r.Unlock()
	return r.addCaller
}

// SetNewCoreFn sets the function that creates the inner zapcore.Cores to which flume forwards logs.
// If not set, flume will use zapcore.NewCore.  This is mainly useful for integration with other logging
// systems.
func (r *Factory) SetNewCoreFn(f func(name string, encoder zapcore.Encoder, out zapcore.WriteSyncer, levelEnabler zapcore.LevelEnabler) zapcore.Core) {
	r.Lock()
	defer r.Unlock()
	r.newCoreFn = f
	r.refreshLoggers()
}

func (r *Factory) getOut() io.Writer {
	if r.out == nil {
		return os.Stdout
	}
	return r.out
}

func (r *Factory) refreshLoggers() {
	for name, info := range r.loggers {
		info.atomicInnerCore.set(r.newInnerCore(name, info))
	}
}

func (r *Factory) getLoggerInfo(name string) *loggerInfo {
	info, found := r.loggers[name]
	if !found {
		info = &loggerInfo{}
		r.loggers[name] = info
		info.atomicInnerCore.set(r.newInnerCore(name, info))
	}
	return info
}

func (r *Factory) newInnerCore(name string, info *loggerInfo) *innerCore {
	var l zapcore.LevelEnabler
	switch {
	case info.levelEnabler != nil:
		l = info.levelEnabler
	default:
		l = r.defaultLevel
	}

	var zcore zapcore.Core
	if r.newCoreFn != nil {
		zcore = r.newCoreFn(name, r.getEncoder(), zapcore.AddSync(r.getOut()), l)
	} else {
		zcore = zapcore.NewCore(r.getEncoder(), zapcore.AddSync(r.getOut()), l)
	}

	return &innerCore{
		name:        name,
		Core:        zcore,
		addCaller:   r.addCaller,
		errorOutput: zapcore.AddSync(os.Stderr),
		hooks:       r.hooks,
	}
}

// NewLogger returns a new Logger
func (r *Factory) NewLogger(name string) Logger {
	return r.NewCore(name)
}

// NewCore returns a new Core.
func (r *Factory) NewCore(name string, options ...CoreOption) *Core {
	r.Lock()
	defer r.Unlock()
	info := r.getLoggerInfo(name)
	core := &Core{
		atomicInnerCore: &info.atomicInnerCore,
	}
	for _, opt := range options {
		opt.apply(core)
	}
	return core
}

func (r *Factory) setLevel(name string, l Level) {
	info := r.getLoggerInfo(name)
	info.levelEnabler = zapcore.Level(l)
}

// SetLevel sets the log level for a particular named logger.  All loggers with this same
// are affected, in the past or future.
func (r *Factory) SetLevel(name string, l Level) {
	r.Lock()
	defer r.Unlock()
	r.setLevel(name, l)
	r.refreshLoggers()
}

// SetDefaultLevel sets the default log level for all loggers which don't have a specific level
// assigned to them
func (r *Factory) SetDefaultLevel(l Level) {
	r.defaultLevel.SetLevel(zapcore.Level(l))
}

type Entry = zapcore.Entry
type CheckedEntry = zapcore.CheckedEntry
type Field = zapcore.Field

// HookFunc adapts a single function to the Hook interface.
type HookFunc func(*CheckedEntry, []Field) []Field

// Hooks adds functions which are called before a log entry is encoded.  The hook function
// is given the entry and the total set of fields to be logged.  The set of fields which are
// returned are then logged.  Hook functions can return a modified set of fields, or just return
// the unaltered fields.
//
// The Entry is not modified.  It is purely informational.
//
// If a hook returns an error, that error is logged, but the in-flight log entry
// will proceed with the original set of fields.
//
// These global hooks will be injected into all loggers owned by this factory.  They will
// execute before any hooks installed in individual loggers.
func (r *Factory) Hooks(hooks ...HookFunc) {
	r.Lock()
	defer r.Unlock()
	r.hooks = append(r.hooks, hooks...)
	r.refreshLoggers()
}

// ClearHooks removes all hooks.
func (r *Factory) ClearHooks() {
	r.Lock()
	defer r.Unlock()
	r.hooks = nil
	r.refreshLoggers()
}

func parseConfigString(s string) map[string]interface{} {
	if s == "" {
		return nil
	}
	items := strings.Split(s, ",")
	m := map[string]interface{}{}
	for _, setting := range items {
		parts := strings.Split(setting, "=")

		switch len(parts) {
		case 1:
			name := parts[0]
			if strings.HasPrefix(name, "-") {
				m[name[1:]] = false
			} else {
				m[name] = true
			}
		case 2:
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// LevelsString reconfigures the log level for all loggers.  Calling it with
// an empty string will reset the default level to info, and reset all loggers
// to use the default level.
//
// The string can contain a list of directives, separated by commas.  Directives
// can set the default log level, and can explicitly set the log level for individual
// loggers.
//
// # Directives
//
// - Default level: Use the `*` directive to set the default log level.  Examples:
//
//   - // set the default log level to debug
//     -* // set the default log level to off
//
//     If the `*` directive is omitted, the default log level will be set to info.
//
//   - Logger level: Use the name of the logger to set the log level for a specific
//     logger.  Examples:
//
//     http		// set the http logger to debug
//     -http		// set the http logger to off
//     http=INF	// set the http logger to info
//
// Multiple directives can be included, separated by commas. Examples:
//
//	http         	// set http logger to debug
//	http,sql     	// set http and sql logger to debug
//	*,-http,sql=INF	// set the default level to debug, disable the http logger,
//	                 // and set the sql logger to info
func (r *Factory) LevelsString(s string) error {
	m := parseConfigString(s)
	levelMap := map[string]Level{}
	var errMsgs []string
	for key, val := range m {
		switch t := val.(type) {
		case bool:
			if t {
				levelMap[key] = DebugLevel
			} else {
				levelMap[key] = OffLevel
			}
		case string:
			l, err := levelForAbbr(t)
			levelMap[key] = l
			if err != nil {
				errMsgs = append(errMsgs, err.Error())
			}
		}
	}
	// first, check default setting
	if defaultLevel, found := levelMap["*"]; found {
		r.SetDefaultLevel(defaultLevel)
		delete(levelMap, "*")
	} else {
		r.SetDefaultLevel(InfoLevel)
	}

	r.Lock()
	defer r.Unlock()

	// iterate through the current level map first.
	// Any existing loggers which aren't in the levels map
	// get reset to the default level.
	for name, info := range r.loggers {
		if _, found := levelMap[name]; !found {
			info.levelEnabler = r.defaultLevel
		}
	}

	// iterate through the levels map and set the specific levels
	for name, level := range levelMap {
		r.setLevel(name, level)
	}

	if len(errMsgs) > 0 {
		return merry.New("errors parsing config string: " + strings.Join(errMsgs, ", "))
	}

	r.refreshLoggers()
	return nil
}

// Configure uses a serializable struct to configure most of the options.
// This is useful when fully configuring the logging from an env var or file.
//
// The zero value for Config will set defaults for a standard, production logger:
//
// See the Config docs for details on settings.
func (r *Factory) Configure(cfg Config) error {

	r.SetDefaultLevel(cfg.DefaultLevel)

	var encCfg *EncoderConfig
	if cfg.EncoderConfig != nil {
		encCfg = cfg.EncoderConfig
	} else {
		if cfg.Development {
			encCfg = NewDevelopmentEncoderConfig()
		} else {
			encCfg = NewEncoderConfig()
		}
	}

	// These *Caller properties *must* be set or errors
	// will occur
	if encCfg.EncodeCaller == nil {
		encCfg.EncodeCaller = zapcore.ShortCallerEncoder
	}
	if encCfg.EncodeLevel == nil {
		encCfg.EncodeLevel = AbbrLevelEncoder
	}

	var encoder zapcore.Encoder
	switch cfg.Encoding {
	case "json":
		encoder = NewJSONEncoder(encCfg)
	case "ltsv":
		encoder = NewLTSVEncoder(encCfg)
	case "term":
		encoder = NewConsoleEncoder(encCfg)
	case "term-color":
		encoder = NewColorizedConsoleEncoder(encCfg, nil)
	case "console":
		encoder = zapcore.NewConsoleEncoder((zapcore.EncoderConfig)(*encCfg))
	case "":
		if cfg.Development {
			encoder = NewColorizedConsoleEncoder(encCfg, nil)
		} else {
			encoder = NewJSONEncoder(encCfg)
		}
	default:
		return merry.Errorf("%s is not a valid encoding, must be one of: json, ltsv, term, or term-color", cfg.Encoding)
	}

	var addCaller bool
	if cfg.AddCaller != nil {
		addCaller = *cfg.AddCaller
	} else {
		addCaller = cfg.Development
	}

	if cfg.Levels != "" {
		if err := r.LevelsString(cfg.Levels); err != nil {
			return err
		}
	}
	r.Lock()
	defer r.Unlock()
	r.encoder = encoder
	r.addCaller = addCaller
	r.refreshLoggers()
	return nil
}

func levelForAbbr(abbr string) (Level, error) {
	switch strings.ToLower(abbr) {
	case "off":
		return OffLevel, nil
	case "dbg", "debug", "", "all":
		return DebugLevel, nil
	case "inf", "info":
		return InfoLevel, nil
	case "err", "error":
		return ErrorLevel, nil
	default:
		return InfoLevel, fmt.Errorf("%s not recognized level, defaulting to info", abbr)
	}
}
