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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package kit for Kubernetes operators
package kit

import "time"

// panicTimer panics when it reaches the given duration.
type panicTimer struct {
	duration time.Duration
	message  string
	timer    *time.Timer
}

func (t *panicTimer) Start() {
	t.timer = time.AfterFunc(t.duration, func() {
		panic(t.message)
	})
}

// stop stops the timer and resets the elapsed duration.
func (t *panicTimer) Stop() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
}
