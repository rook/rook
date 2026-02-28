package flume

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap/zapcore"
	"os"
	"strings"
	"time"
)

// DefaultConfigEnvVars is a list of the environment variables
// that ConfigFromEnv will search by default.
var DefaultConfigEnvVars = []string{"FLUME"}

// ConfigFromEnv configures flume from environment variables.
// It should be called from main():
//
//	func main() {
//	    flume.ConfigFromEnv()
//	    ...
//	 }
//
// It searches envvars for the first environment
// variable that is set, and attempts to parse the value.
//
// If no environment variable is set, it silently does nothing.
//
// If an environment variable with a value is found, but parsing
// fails, an error is printed to stdout, and the error is returned.
//
// If envvars is empty, it defaults to DefaultConfigEnvVars.
func ConfigFromEnv(envvars ...string) error {
	if len(envvars) == 0 {
		envvars = DefaultConfigEnvVars
	}

	var configString string

	for _, v := range envvars {
		configString = os.Getenv(v)
		if configString != "" {
			err := ConfigString(configString)
			if err != nil {
				fmt.Println("error parsing log config from env var " + v + ": " + err.Error())
			}
			return err
		}
	}

	return nil
}

// Config offers a declarative way to configure a Factory.
//
// The same things can be done by calling Factory methods, but
// Configs can be unmarshaled from JSON, making it a convenient
// way to configure most logging options from env vars or files, i.e.:
//
//	err := flume.ConfigString(os.Getenv("flume"))
//
// Configs can be created and applied programmatically:
//
//	err := flume.Configure(flume.Config{})
//
// Defaults are appropriate for a JSON encoded production logger:
//
// - LTSV encoder
// - full timestamps
// - default log level set to INFO
// - call sites are not logged
//
// An alternate set of defaults, more appropriate for development environments,
// can be configured with `Config{Development:true}`:
//
//	err := flume.Configure(flume.Config{Development:true})
//
// - colorized terminal encoder
// - short timestamps
// - call sites are logged
//
//	err := flume.Configure(flume.Config{Development:true})
//
// Any of the other configuration options can be specified to override
// the defaults.
//
// Note: If configuring the EncoderConfig setting, if any of the *Key properties
// are omitted, that entire field will be omitted.
type Config struct {
	// DefaultLevel is the default log level for all loggers not
	// otherwise configured by Levels.  Defaults to Info.
	DefaultLevel Level `json:"level" yaml:"level"`
	// Levels configures log levels for particular named loggers.  See
	// LevelsString for format.
	Levels string `json:"levels" yaml:"levels"`
	// AddCaller annotates logs with the calling function's file
	// name and line number. Defaults to true when the Development
	// flag is set, false otherwise.
	AddCaller *bool `json:"addCaller" yaml:"addCaller"`
	// Encoding sets the logger's encoding. Valid values are "json",
	// "console", "ltsv", "term", and "term-color".
	// Defaults to "term-color" if development is true, else
	// "ltsv"
	Encoding string `json:"encoding" yaml:"encoding"`
	// Development toggles the defaults used for the other
	// settings.  Defaults to false.
	Development bool `json:"development" yaml:"development"`
	// EncoderConfig sets options for the chosen encoder. See
	// EncoderConfig for details.  Defaults to NewEncoderConfig() if
	// Development is false, otherwise defaults to NewDevelopmentEncoderConfig().
	EncoderConfig *EncoderConfig `json:"encoderConfig" yaml:"encoderConfig"`
}

// SetAddCaller sets the Config's AddCaller flag.
func (c *Config) SetAddCaller(b bool) {
	c.AddCaller = &b
}

// UnsetAddCaller unsets the Config's AddCaller flag (reverting to defaults).
func (c *Config) UnsetAddCaller() {
	c.AddCaller = nil
}

// EncoderConfig captures the options for encoders.
// Type alias to avoid exporting zap.
type EncoderConfig zapcore.EncoderConfig

type privEncCfg struct {
	EncodeLevel string `json:"levelEncoder" yaml:"levelEncoder"`
	EncodeTime  string `json:"timeEncoder" yaml:"timeEncoder"`
}

// UnmarshalJSON implements json.Marshaler
func (enc *EncoderConfig) UnmarshalJSON(b []byte) error {
	var zapCfg zapcore.EncoderConfig
	err := json.Unmarshal(b, &zapCfg)
	if err != nil {
		return err
	}
	var pc privEncCfg
	err = json.Unmarshal(b, &pc)
	if err == nil {
		switch pc.EncodeLevel {
		case "", "abbr":
			zapCfg.EncodeLevel = AbbrLevelEncoder
		}
		switch pc.EncodeTime {
		case "":
			zapCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		case "justtime":
			zapCfg.EncodeTime = JustTimeEncoder
		}
	}
	*enc = EncoderConfig(zapCfg)
	return nil
}

// NewEncoderConfig returns an EncoderConfig with default settings.
func NewEncoderConfig() *EncoderConfig {
	return &EncoderConfig{
		MessageKey:     "msg",
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "name",
		CallerKey:      "caller",
		StacktraceKey:  "stacktrace",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeLevel:    AbbrLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
}

// NewDevelopmentEncoderConfig returns an EncoderConfig which is intended
// for local development.
func NewDevelopmentEncoderConfig() *EncoderConfig {
	cfg := NewEncoderConfig()
	cfg.EncodeTime = JustTimeEncoder
	cfg.EncodeDuration = zapcore.StringDurationEncoder
	return cfg
}

// JustTimeEncoder is a timestamp encoder function which encodes time
// as a simple time of day, without a date.  Intended for development and testing.
// Not good in a production system, where you probably need to know the date.
//
//	encConfig := flume.EncoderConfig{}
//	encConfig.EncodeTime = flume.JustTimeEncoder
func JustTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("15:04:05.000"))
}

// AbbrLevelEncoder encodes logging levels to the strings in the log entries.
// Encodes levels as 3-char abbreviations in upper case.
//
//	encConfig := flume.EncoderConfig{}
//	encConfig.EncodeTime = flume.AbbrLevelEncoder
func AbbrLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch l {
	case zapcore.DebugLevel:
		enc.AppendString("DBG")
	case zapcore.InfoLevel:
		enc.AppendString("INF")
	case zapcore.WarnLevel:
		enc.AppendString("WRN")
	case zapcore.ErrorLevel:
		enc.AppendString("ERR")
	case zapcore.PanicLevel, zapcore.FatalLevel, zapcore.DPanicLevel:
		enc.AppendString("FTL")
	default:
		s := l.String()
		if len(s) > 3 {
			s = s[:3]
		}
		enc.AppendString(strings.ToUpper(s))

	}
}
