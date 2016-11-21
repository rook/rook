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
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "api")

func Logger(inner http.Handler, name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		inner.ServeHTTP(w, r)

		logger.Infof(
			"(%s)\t%s\t%s\t%s",
			time.Since(start),
			r.Method,
			r.RequestURI,
			name,
		)
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
