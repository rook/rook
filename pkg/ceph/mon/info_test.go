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
package mon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractKey(t *testing.T) {
	contents := "bad key"
	key, err := extractKey(contents)
	assert.NotNil(t, err)
	assert.Equal(t, "", key)

	contents = `[mon.]
	key = AQCboyRZdrgXFBAAtqrTAM1Wf09v5hBbiLeBdQ==
	caps mon = "'allow *'"
`
	key, err = extractKey(contents)
	assert.Nil(t, err)
	assert.Equal(t, "AQCboyRZdrgXFBAAtqrTAM1Wf09v5hBbiLeBdQ==", key)

}
