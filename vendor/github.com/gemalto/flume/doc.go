// Package flume is a logging package, build on top of zap.  It's structured and leveled logs, like zap/logrus/etc.
// It adds global, runtime re-configuration of all loggers, via an internal logger registry.
//
// There are two interaction points with flume: code that generates logs, and code that configures logging output.
// Code which generates logs needs to create named logger instances, and call log functions on it, like Info()
// and Debug().  But by default, all these logs will be silently discarded.  Flume does not output
// log entries unless explicitly told to do so.  This ensures libraries can freely use flume internally, without
// polluting the stdout of the programs importing the library.
//
// The Logger type is a small interface.  Libraries should allow replacement of their Logger instances so
// importers can entirely replace flume if they wish.  Alternately, importers can use flume to configure
// the library's log output, and/or redirect it into the overall program's log stream.
//
// # Logging
//
// This package does not offer package level log functions, so you need to create a logger instance first:
// A common pattern is to create a single, package-wide logger, named after the package:
//
//	var log = flume.New("mypkg")
//
// Then, write some logs:
//
//	log.Debug("created user", "username", "frank", "role", "admin")
//
// Logs have a message, then matched pairs of key/value properties.  Child loggers can be created
// and pre-seeded with a set of properties:
//
//	reqLogger := log.With("remoteAddr", req.RemoteAddr)
//
// Expensive log events can be avoid by explicitly checking level:
//
//	if log.IsDebug() {
//	    log.Debug("created resource", "resource", resource.ExpensiveToString())
//	}
//
// Loggers can be bound to context.Context, which is convenient for carrying
// per-transaction loggers (pre-seeded with transaction specific context) through layers of request
// processing code:
//
//	ctx = flume.WithLogger(ctx, log.With("transactionID", tid))
//	// ...later...
//	flume.FromContext(ctx).Info("Request handled.")
//
// The standard Logger interface only supports 3 levels of log, DBG, INF, and ERR.  This is inspired by
// this article: https://dave.cheney.net/2015/11/05/lets-talk-about-logging.  However, you can create
// instances of DeprecatedLogger instead, which support more levels.
//
// # Configuration
//
// There are several package level functions which reconfigure logging output.  They control which
// levels are discarded, which fields are included in each log entry, and how those fields are rendered,
// and how the overall log entry is rendered (JSON, LTSV, colorized, etc).
//
// To configure logging settings from environment variables, call the configuration function from main():
//
//	flume.ConfigFromEnv()
//
// This reads the log configuration from the environment variable "FLUME" (the default, which can be
// overridden).  The value is JSON, e.g.:
//
//	{"level":"INF","levels":"http=DBG","development"="true"}
//
// The properties of the config string:
//
//   - "level": ERR, INF, or DBG.  The default level for all loggers.
//
//   - "levels": A string configuring log levels for specific loggers, overriding the default level.
//     See note below for syntax.
//
//   - "development": true or false.  In development mode, the defaults for the other
//     settings change to be more suitable for developers at a terminal (colorized, multiline, human
//     readable, etc).  See note below for exact defaults.
//
//   - "addCaller": true or false.  Adds call site information to log entries (file and line).
//
//   - "encoding": json, ltsv, term, or term-color.  Configures how log entries are encoded in the output.
//     "term" and "term-color" are multi-line, human-friendly
//     formats, intended for terminal output.
//
//   - "encoderConfig": a JSON object which configures advanced encoding settings, like how timestamps
//     are formatted.  See docs for go.uber.org/zap/zapcore/EncoderConfig
//
//   - "messageKey": the label of the message property of the log entry.  If empty, message is omitted.
//
//   - "levelKey": the label of the level property of the log entry.  If empty, level is omitted.
//
//   - "timeKey": the label of the timestamp of the log entry.  If empty, timestamp is omitted.
//
//   - "nameKey": the label of the logger name in the log entry.  If empty, logger name is omitted.
//
//   - "callerKey": the label of the logger name in the log entry.  If empty, logger name is omitted.
//
//   - "lineEnding": the end of each log output line.
//
//   - "levelEncoder": capital, capitalColor, color, lower, or abbr.  Controls how the log entry level
//     is rendered.  "abbr" renders 3-letter abbreviations, like ERR and INF.
//
//   - "timeEncoder": iso8601, millis, nanos, unix, or justtime.  Controls how timestamps are rendered.
//     "millis", "nanos", and "unix" are since UNIX epoch.  "unix" is in floating point seconds.
//     "justtime" omits the date, and just prints the time in the format "15:04:05.000".
//
//   - "durationEncoder": string, nanos, or seconds.  Controls how time.Duration values are rendered.
//
//   - "callerEncoder": full or short.  Controls how the call site is rendered.
//     "full" includes the entire package path, "short" only includes the last folder of the package.
//
// Defaults:
//
//	{
//	  "level":"INF",
//	  "levels":"",
//	  "development":false,
//	  "addCaller":false,
//	  "encoding":"term-color",
//	  "encoderConfig":{
//	    "messageKey":"msg",
//	    "levelKey":"level",
//	    "timeKey":"time",
//	    "nameKey":"name",
//	    "callerKey":"caller",
//	    "lineEnding":"\n",
//	    "levelEncoder":"abbr",
//	    "timeEncoder":"iso8601",
//	    "durationEncoder":"seconds",
//	    "callerEncoder":"short",
//	  }
//	}
//
// These defaults are only applied if one of the configuration functions is called, like ConfigFromEnv(), ConfigString(),
// Configure(), or LevelsString().  Initially, all loggers are configured to discard everything, following
// flume's opinion that log packages should be silent unless spoken too.  Ancillary to this: library packages
// should *not* call these functions, or configure logging levels or output in anyway.  Only program entry points,
// like main() or test code, should configure logging.  Libraries should just create loggers and log to them.
//
// Development mode: if "development"=true, the defaults for the rest of the settings change, equivalent to:
//
//	{
//	  "addCaller":true,
//	  "encoding":"term-color",
//	  "encodingConfig": {
//	    "timeEncoder":"justtime",
//	    "durationEncoder":"string",
//	  }
//	}
//
// The "levels" value is a list of key=value pairs, configuring the level of individual named loggers.
// If the key is "*", it sets the default level.  If "level" and "levels" both configure the default
// level, "levels" wins.
// Examples:
//
//   - // set the default level to ALL, equivalent to {"level"="ALL"}
//     *=INF		// same, but set default level to INF
//     *,sql=WRN	// set default to ALL, set "sql" logger to WRN
//     *=INF,http=ALL	// set default to INF, set "http" to ALL
//     *=INF,http	// same as above.  If name has no level, level is set to ALL
//     *=INF,-http	// set default to INF, set "http" to OFF
//     http=INF		// leave default setting unchanged.
//
// # Factories
//
// Most usages of flume will use its package functions.  The package functions delegate to an internal
// instance of Factory, which a the logger registry.  You can create and manage your own instance of
// Factory, which will be an isolated set of Loggers.
//
// tl;dr
//
// The implementation is a wrapper around zap.   zap does levels, structured logs, and is very fast.
// zap doesn't do centralized, global configuration, so this package
// adds that by maintaining an internal registry of all loggers, and using the sync.atomic stuff to swap out
// levels and writers in a thread safe way.
package flume
