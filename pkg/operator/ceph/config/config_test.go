/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Everything here is for the ultimate purpose of generating ini config files, and this is the main
// output. Testing other functions would require this for testing output anyway, so just test all
// the functions here.
func TestConfig_FileText(t *testing.T) {
	c := NewConfig()

	iniText := func() string {
		f, e := c.IniFile()
		assert.NoError(t, e)
		b := new(bytes.Buffer)
		_, e = f.WriteTo(b)
		assert.NoError(t, e)
		return b.String()
	}

	// Empty config
	f := iniText()
	assert.Equal(t, "", f)

	// single section, empty
	c.Section("global")
	f = iniText()
	assert.Equal(t, "[global]\n\n", f)

	// two sections, both empty
	c.Section("osd")
	f = iniText()
	assert.Equal(t, "[global]\n\n[osd]\n\n", f)

	// add configs to empty global section
	c.Section("global").
		Set("key", "val").
		Set("key with spaces", "my-val").
		Set("key_with_underscores", "my val").
		Set("key-with-hyphens", "\"my_val \"")
	f = iniText()
	assert.Equal(t, `[global]
key                  = val
key_with_spaces      = my-val
key_with_underscores = my val
key_with_hyphens     = "my_val "

[osd]

`, f)

	// change some of the global options to make sure the keys override properly
	c.Section("global").
		Set("key", "val"). // do not change this val to test this corner case
		Set("key_with_spaces", "my-val2").
		Set("key with underscores", "my val2")
	// add new section with a config set
	c.Section("mon").Set("debug mon", "100")
	f = iniText()
	assert.Equal(t, `[global]
key                  = val
key_with_spaces      = my-val2
key_with_underscores = my val2
key_with_hyphens     = "my_val "

[osd]

[mon]
debug_mon = 100

`, f)

	// test merging by creating a new config and merging it with the previous one
	// config merging uses section merging as part of it, so no need to test section merging
	m := NewConfig()
	m.Section("global"). // preexisting section
				Set("key", "val").                // preexisting key overridden w/ same val
				Set("key with spaces", "my-val"). // preexisting key overridden w/ diff val
				Set("new key", "new")             // new key
	m.Section("mgr"). // new section
				Set("debug mgr", "100") // new val
	c.Merge(m)
	f = iniText()
	assert.Equal(t, `[global]
key                  = val
key_with_spaces      = my-val
key_with_underscores = my val2
key_with_hyphens     = "my_val "
new_key              = new

[osd]

[mon]
debug_mon = 100

[mgr]
debug_mgr = 100

`, f)
}

func TestNewFlag(t *testing.T) {
	assert.Equal(t, NewFlag("k", ""), "--k=")
	assert.Equal(t, NewFlag("a-key", "a"), "--a-key=a")
	assert.Equal(t, NewFlag("b_key", "b"), "--b-key=b")
	assert.Equal(t, NewFlag("c key", "c"), "--c-key=c")
	assert.Equal(t, NewFlag("quotes", "\"quoted\""), "--quotes=\"quoted\"")
}

func TestConfig_GlobalFlags(t *testing.T) {
	c := NewConfig()
	assert.Equal(t, c.GlobalFlags(), []string{})

	c.Section("global").
		Set("test key", "one").
		Set("two_key", "2").
		Set("3-key", "\"trois \"")
	assert.ElementsMatch(t, []string{"--test-key=one", "--two-key=2", "--3-key=\"trois \""}, c.GlobalFlags())

	c.Section("mon").
		Set("mon key", "m")
	assert.ElementsMatch(t, []string{"--test-key=one", "--two-key=2", "--3-key=\"trois \""}, c.GlobalFlags())
}
