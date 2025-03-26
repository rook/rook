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
package display

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBytesToString(t *testing.T) {
	// one value for each unit
	assert.Equal(t, "1 B", BytesToString(1))
	assert.Equal(t, "1.00 KiB", BytesToString(1024))
	assert.Equal(t, "2.22 MiB", BytesToString(2327839))
	assert.Equal(t, "3.33 GiB", BytesToString(3575560274))
	assert.Equal(t, "4.44 TiB", BytesToString(4881831627325))
	assert.Equal(t, "5.55 PiB", BytesToString(6248744482976563))
	assert.Equal(t, "6.66 EiB", BytesToString(7678457220681600860))

	// min and max values
	assert.Equal(t, "0 B", BytesToString(0))
	assert.Equal(t, "16.00 EiB", BytesToString(math.MaxUint64))
	assert.Equal(t, uint64(50), BToMb(uint64(52428800)))
	assert.Equal(t, uint64(52428800), MbTob(uint64(50)))
}
