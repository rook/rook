flume [![GoDoc](https://godoc.org/github.com/gemalto/flume?status.png)](https://godoc.org/github.com/gemalto/flume) [![Go Report Card](https://goreportcard.com/badge/github.com/gemalto/flume)](https://goreportcard.com/report/gemalto/flume) [![Build](https://github.com/gemalto/flume/workflows/Build/badge.svg)](https://github.com/gemalto/flume/actions?query=branch%3Amaster+workflow%3ABuild+)
=====

> ### [Flume v2](https://github.com/ThalesGroup/flume/tree/master/v2) is near release.  This is a ground up rewrite of flume as a slog.Handler.
> v1 will continue to be supported, but new applications should consider slog and v2.

flume is a logging package, build on top of [zap](https://github.com/uber-go/zap).  It's structured and leveled logs, like zap/logrus/etc.
It adds a global registry of all loggers, allowing global re-configuration at runtime.   Instantiating
new loggers automatically registers them: even loggers created in init() functions, package variable
initializers, or 3rd party code, can all be managed from the central registry.

Features

- Structured: Log entries have key/value attributes.
- Leveled:

      - Error: Something that would be reported up to an error reporting service
      - Info: High priority, low volume messages. Appropriate for production runtime use.  Used for coarse-grained
        feedback
      - Debug: Slow, verbose logging, not appropriate for long-term production use

  Flume is a little opinionated about having only a few logs levels.  Warns should either be errors
  or infos, trace should just be debug, and a log package shouldn't be responsible for panics or exits.
- Named: Loggers have a name.  Levels can be configured for each named logger.  For example, a common usage
  pattern is to create a unique logger for each go package, then selectively turn on debug logging for
  specific packages.
- Built on top of zap, which is super fast.
- Supports JSON, LTSV, and colorized console output formats.
- Optional call site logging (file and line number of log call)
- Output can be directed to any writer, defaults to stdout
- Helpers for managing application logs during tests
- Supports creating child loggers with pre-set context: `Logger.With()`
- Levels can be configured via a single string, which is convenient for configuration via env var, see `LevelsString()`
- All loggers can be reconfigured dynamically at runtime.
- Thoughtful handling of multi-line log output: Multi-line output is collapsed to a single line, or encoded,
  depending on the encoder.  The terminal encoders, which are optimized for human viewing, retain multi-line
  formatting.
- By default, all logs are discarded.  Flume is completely silent unless explicitly configured otherwise.
  This is ideal for logging inside libraries, where the log level and output will be managed by
  the code importing the library.

This package does not offer package level log functions, so you need to create a logger instance first:
A common pattern is to create a single, package-wide logger, named after the package:

    var log = flume.New("mypkg")
    
Then, write some logs:

    log.Debug("created user", "username", "frank", "role", "admin")
    
Logs have a message, then matched pairs of key/value properties.  Child loggers can be created
and pre-seeded with a set of properties:

    reqLogger := log.With("remoteAddr", req.RemoteAddr)
    
Expensive log events can be avoid by explicitly checking level:

    if log.IsDebug() {
        log.Debug("created resource", "resource", resource.ExpensiveToString())
    }
    
Loggers can be bound to context.Context, which is convenient for carrying
per-transaction loggers (pre-seeded with transaction specific context) through layers of request
processing code:

    ctx = flume.WithLogger(ctx, log.With("transactionID", tid))
    // ...later...
    flume.FromContext(ctx).Info("Request handled.")

By default, all these messages will simply be discard.  To enable output, flume needs to
be configured.  Only entry-point code, like main() or test setup, should configure flume.

To configure logging settings from environment variables, call the configuration function from main():

    flume.ConfigFromEnv()
    
Other configuration methods are available: see `ConfigString()`, `LevelString()`, and `Configure()`.

This reads the log configuration from the environment variable "FLUME" (the default, which can be
overridden).  The value is JSON, e.g.:

    {"level":"INF","levels":"http=DBG","development"="true"}

The properties of the config string:

    - "level": ERR, INF, or DBG.  The default level for all loggers.
    - "levels": A string configuring log levels for specific loggers, overriding the default level.
      See note below for syntax.
    - "development": true or false.  In development mode, the defaults for the other
      settings change to be more suitable for developers at a terminal (colorized, multiline, human
      readable, etc).  See note below for exact defaults.
    - "addCaller": true or false.  Adds call site information to log entries (file and line).
    - "encoding": json, ltsv, term, or term-color.  Configures how log entries are encoded in the output.
      "term" and "term-color" are multi-line, human-friendly
      formats, intended for terminal output.
    - "encoderConfig": a JSON object which configures advanced encoding settings, like how timestamps
      are formatted.  See docs for go.uber.org/zap/zapcore/EncoderConfig

        - "messageKey": the label of the message property of the log entry.  If empty, message is omitted.
        - "levelKey": the label of the level property of the log entry.  If empty, level is omitted.
        - "timeKey": the label of the timestamp of the log entry.  If empty, timestamp is omitted.
        - "nameKey": the label of the logger name in the log entry.  If empty, logger name is omitted.
        - "callerKey": the label of the logger name in the log entry.  If empty, logger name is omitted.
        - "stacktraceKey": the label of the stacktrace in the log entry.  If empty, stacktrace is omitted.
        - "lineEnding": the end of each log output line.
        - "levelEncoder": capital, capitalColor, color, lower, or abbr.  Controls how the log entry level
          is rendered.  "abbr" renders 3-letter abbreviations, like ERR and INF.
        - "timeEncoder": iso8601, millis, nanos, unix, or justtime.  Controls how timestamps are rendered.
			 "millis", "nanos", and "unix" are since UNIX epoch.  "unix" is in floating point seconds.
          "justtime" omits the date, and just prints the time in the format "15:04:05.000".
        - "durationEncoder": string, nanos, or seconds.  Controls how time.Duration values are rendered.
        - "callerEncoder": full or short.  Controls how the call site is rendered.
          "full" includes the entire package path, "short" only includes the last folder of the package.

Defaults:

    {
      "level":"INF",
      "levels":"",
      "development":false,
      "addCaller":false,
      "encoding":"term-color",
      "encoderConfig":nil
    }

If "encoderConfig" is omitted, it defaults to:

    {
      "messageKey":"msg",
      "levelKey":"level",
      "timeKey":"time",
      "nameKey":"name",
      "callerKey":"caller",
      "stacktraceKey":"stacktrace",
      "lineEnding":"\n",
      "levelEncoder":"abbr",
      "timeEncoder":"iso8601",
      "durationEncoder":"seconds",
      "callerEncoder":"short",
    }

These defaults are only applied if one of the configuration functions is called, like ConfigFromEnv(), ConfigString(),
Configure(), or LevelsString().  Initially, all loggers are configured to discard everything, following
flume's opinion that log packages should be silent unless spoken too.  Ancillary to this: library packages
should *not* call these functions, or configure logging levels or output in anyway.  Only program entry points,
like main() or test code, should configure logging.  Libraries should just create loggers and log to them.

Development mode: if "development"=true, the defaults for the rest of the settings change, equivalent to:

    {
      "addCaller":true,
      "encoding":"term-color",
      "encodingConfig": {
        "timeEncoder":"justtime",
        "durationEncoder":"string",
      }
    }

The "levels" value is a list of key=value pairs, configuring the level of individual named loggers.
If the key is "*", it sets the default level.  If "level" and "levels" both configure the default
level, "levels" wins.
Examples:

    *            // set the default level to ALL, equivalent to {"level"="ALL"}
    *=INF		// same, but set default level to INF
    *,sql=WRN	// set default to ALL, set "sql" logger to WRN
    *=INF,http=ALL	// set default to INF, set "http" to ALL
    *=INF,http	// same as above.  If name has no level, level is set to ALL
    *=INF,-http	// set default to INF, set "http" to OFF
    http=INF		// leave default setting unchanged.
    
Examples of log output:

"term"

    11:42:08.126 INF | Hello World!  	@:root@flume.git/example_test.go:15
    11:42:08.127 INF | This entry has properties  	color:red	@:root@flume.git/example_test.go:16
    11:42:08.127 DBG | This is a debug message  	@:root@flume.git/example_test.go:17
    11:42:08.127 ERR | This is an error message  	@:root@flume.git/example_test.go:18
    11:42:08.127 INF | This message has a multiline value  	essay:
    Four score and seven years ago
    our fathers brought forth on this continent, a new nation, 
    conceived in Liberty, and dedicated to the proposition that all men are created equal.
    @:root@flume.git/example_test.go:19
    
"term-color"

![term-color sample](sample.png)
    
"json"

    {"level":"INF","time":"15:06:28.422","name":"root","caller":"flume.git/example_test.go:15","msg":"Hello World!"}
    {"level":"INF","time":"15:06:28.423","name":"root","caller":"flume.git/example_test.go:16","msg":"This entry has properties","color":"red"}
    {"level":"DBG","time":"15:06:28.423","name":"root","caller":"flume.git/example_test.go:17","msg":"This is a debug message"}
    {"level":"ERR","time":"15:06:28.423","name":"root","caller":"flume.git/example_test.go:18","msg":"This is an error message"}
    {"level":"INF","time":"15:06:28.423","name":"root","caller":"flume.git/example_test.go:19","msg":"This message has a multiline value","essay":"Four score and seven years ago\nour fathers brought forth on this continent, a new nation, \nconceived in Liberty, and dedicated to the proposition that all men are created equal."}
    
"ltsv"

    level:INF	time:15:06:55.325	msg:Hello World!	name:root	caller:flume.git/example_test.go:15	
    level:INF	time:15:06:55.325	msg:This entry has properties	name:root	caller:flume.git/example_test.go:16	color:red
    level:DBG	time:15:06:55.325	msg:This is a debug message	name:root	caller:flume.git/example_test.go:17	
    level:ERR	time:15:06:55.325	msg:This is an error message	name:root	caller:flume.git/example_test.go:18	
    level:INF	time:15:06:55.325	msg:This message has a multiline value	name:root	caller:flume.git/example_test.go:19	essay:Four score and seven years ago\nour fathers brought forth on this continent, a new nation, \nconceived in Liberty, and dedicated to the proposition that all men are created equal.
    
tl;dr

The implementation is a wrapper around zap.   zap does levels, structured logs, and is very fast.
zap doesn't do centralized, global configuration, so this package
adds that by maintaining an internal registry of all loggers, and using the sync.atomic stuff to swap out
levels and writers in a thread safe way.

Contributing
------------

To build, be sure to have a recent go SDK, and make.  Run `make tools` to install other dependencies.  Then run `make`.

There is also a dockerized build, which only requires make and docker-compose: `make docker`.  You can also
do `make fish` or `make bash` to shell into the docker build container.

Merge requests are welcome!  Before submitting, please run `make` and make sure all tests pass and there are
no linter findings.