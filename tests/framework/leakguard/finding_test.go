/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package leakguard

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretFormattingNeverContainsValue(t *testing.T) {
	s := Secret{Source: "cephx", Name: "osd.1", Value: osdKey}

	outputs := map[string]string{
		"String":   s.String(),
		"GoString": s.GoString(),
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		outputs[verb] = fmt.Sprintf(verb, s)
		outputs["pointer "+verb] = fmt.Sprintf(verb, &s)
	}

	for how, out := range outputs {
		assert.NotContains(t, out, osdKey, how)
		assert.Contains(t, out, "cephx", how)
		assert.Contains(t, out, "osd.1", how)
	}
}
