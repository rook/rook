/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package api

import (
	"net/http"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/google/uuid"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "api")

// Implements the http.ResponseWriter interface to intercept response information
// in order to log it later.
type loggerResponseWriter struct {
	innerWriter http.ResponseWriter
	status      int
}

func newLoggerResponseWriter(w http.ResponseWriter) *loggerResponseWriter {
	return &loggerResponseWriter{
		innerWriter: w,
		status:      http.StatusOK,
	}
}

func (w *loggerResponseWriter) Header() http.Header {
	return w.innerWriter.Header()
}

func (w *loggerResponseWriter) Write(d []byte) (int, error) {
	return w.innerWriter.Write(d)
}

func (w *loggerResponseWriter) WriteHeader(status int) {
	w.status = status
	w.innerWriter.WriteHeader(status)
}

func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID, _ := uuid.NewRandom()

		// logs the start of an API request/response in the form of:
		//	e66d0e38-155b-4179-b343-e3881cd80386	GET	/status	GetStatusDetails	(start)
		logger.Infof("%s\t%s\t%s\t%s\t(start)", reqID.String(), r.Method, r.RequestURI, name)

		// wrap the response writer with a loggerResponseWriter to catch details for logging that
		// are written in the response
		lrw := newLoggerResponseWriter(w)

		// perform the inner ServeHTTP
		inner.ServeHTTP(lrw, r)

		// logs the end of an API request/response in the form of:
		// 	e66d0e38-155b-4179-b343-e3881cd80386	GET	/status	GetStatusDetails	200 OK	10.012447ms
		logger.Infof("%s\t%s\t%s\t%s\t%d %s\t%s", reqID.String(), r.Method, r.RequestURI, name,
			lrw.status, http.StatusText(lrw.status), time.Since(start))
	})
}

// Sets the log level for this node.
// POST
// /log
func (h *Handler) SetLogLevel(w http.ResponseWriter, r *http.Request) {
	l, ok := r.URL.Query()["level"]
	if !ok || l[0] == "" {
		http.Error(w, "log level not passed", http.StatusBadRequest)
		return
	}

	logLevel, err := capnslog.ParseLevel(l[0])
	if err != nil {
		http.Error(w, "invalid log level", http.StatusBadRequest)
		return
	}

	capnslog.SetGlobalLogLevel(logLevel)
	w.WriteHeader(http.StatusOK)
}
