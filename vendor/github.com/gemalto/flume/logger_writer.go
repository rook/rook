package flume

import (
	"io"
	"strings"
)

// LogFuncWriter is a writer which writes to a logging function signature
// like that of testing.T.Log() and fmt/log.Println().
// It can be used to redirect flumes *output* to some other logger.
//
//	SetOut(LogFuncWriter(fmt.Println, true))
//	SetOut(LogFuncWriter(t.Log, true))
func LogFuncWriter(l func(args ...interface{}), trimSpace bool) io.Writer {
	return &logWriter{lf: l, trimSpace: trimSpace}
}

// LoggerFuncWriter is a writer which writes lines to a logging function with
// a signature like that of flume.Logger's functions, like Info(), Debug(), and Error().
//
//	http.Server{
//	    ErrorLog: log.New(LoggerFuncWriter(flume.New("http").Error), "", 0),
//	}
func LoggerFuncWriter(l func(msg string, kvpairs ...interface{})) io.Writer {
	return &loggerWriter{lf: l}
}

type logWriter struct {
	lf        func(args ...interface{})
	trimSpace bool
}

// Write implements io.Writer
func (t *logWriter) Write(p []byte) (n int, err error) {
	s := string(p)
	if t.trimSpace {
		s = strings.TrimSpace(s)
	}
	t.lf(s)
	return len(p), nil
}

type loggerWriter struct {
	lf func(msg string, kvpairs ...interface{})
}

// Write implements io.Writer
func (t *loggerWriter) Write(p []byte) (n int, err error) {
	t.lf(string(p))
	return len(p), nil
}
