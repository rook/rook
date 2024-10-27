/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestCephLabelsMerge(t *testing.T) {
	// No Labels defined
	testLabels := LabelsSpec{}
	a := GetOSDLabels(testLabels)
	assert.Nil(t, a)

	// Only a specific component labels without "all"
	testLabels = LabelsSpec{
		"mgr":       {"mgrkey": "mgrval"},
		"mon":       {"monkey": "monval"},
		"osd":       {"osdkey": "osdval"},
		"rgw":       {"rgwkey": "rgwval"},
		"rbdmirror": {"rbdmirrorkey": "rbdmirrorval"},
	}
	a = GetMgrLabels(testLabels)
	assert.Equal(t, "mgrval", a["mgrkey"])
	assert.Equal(t, 1, len(a))
	a = GetMonLabels(testLabels)
	assert.Equal(t, "monval", a["monkey"])
	assert.Equal(t, 1, len(a))
	a = GetOSDLabels(testLabels)
	assert.Equal(t, "osdval", a["osdkey"])
	assert.Equal(t, 1, len(a))

	// No Labels matching the component
	testLabels = LabelsSpec{
		"mgr": {"mgrkey": "mgrval"},
	}
	a = GetMonLabels(testLabels)
	assert.Nil(t, a)

	// Merge with "all"
	testLabels = LabelsSpec{
		"all":         {"allkey1": "allval1", "allkey2": "allval2"},
		"mgr":         {"mgrkey": "mgrval"},
		"cmdreporter": {"detect": "myversion"},
	}
	a = GetMonLabels(testLabels)
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 2, len(a))
	a = GetMgrLabels(testLabels)
	assert.Equal(t, "mgrval", a["mgrkey"])
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 3, len(a))
	a = GetCmdReporterLabels(testLabels)
	assert.Equal(t, "myversion", a["detect"])
	assert.Equal(t, "allval1", a["allkey1"])
	assert.Equal(t, "allval2", a["allkey2"])
	assert.Equal(t, 3, len(a))
}

func TestLabelsSpec(t *testing.T) {
	specYaml := []byte(`
mgr:
  foo: bar
  hello: world
mon:
`)

	// convert the raw spec yaml into JSON
	rawJSON, err := yaml.ToJSON(specYaml)
	assert.Nil(t, err)

	// unmarshal the JSON into a strongly typed Labels spec object
	var Labels LabelsSpec
	err = json.Unmarshal(rawJSON, &Labels)
	assert.Nil(t, err)

	// the unmarshalled Labels spec should equal the expected spec below
	expected := LabelsSpec{
		"mgr": map[string]string{
			"foo":   "bar",
			"hello": "world",
		},
		"mon": nil,
	}
	assert.Equal(t, expected, Labels)
}
func TestLabelsApply(t *testing.T) {
	tcs := []struct {
		name     string
		target   *metav1.ObjectMeta
		input    Labels
		expected Labels
	}{
		{
			name:   "it should be able to update meta with no label",
			target: &metav1.ObjectMeta{},
			input: Labels{
				"foo": "bar",
			},
			expected: Labels{
				"foo": "bar",
			},
		},
		{
			name: "it should keep the original labels when new labels are set",
			target: &metav1.ObjectMeta{
				Labels: Labels{
					"foo": "bar",
				},
			},
			input: Labels{
				"hello": "world",
			},
			expected: Labels{
				"foo":   "bar",
				"hello": "world",
			},
		},
		{
			name: "it should NOT overwrite the existing keys",
			target: &metav1.ObjectMeta{
				Labels: Labels{
					"foo": "bar",
				},
			},
			input: Labels{
				"foo": "baz",
			},
			expected: Labels{
				"foo": "bar",
			},
		},
	}

	for _, tc := range tcs {
		tc.input.ApplyToObjectMeta(tc.target)
		assert.Equal(t, map[string]string(tc.expected), tc.target.Labels)
	}
}

func TestLabelsOverwriteApply(t *testing.T) {
	tcs := []struct {
		name     string
		target   *metav1.ObjectMeta
		input    Labels
		expected Labels
	}{
		{
			name:   "it should be able to update meta with no label",
			target: &metav1.ObjectMeta{},
			input: Labels{
				"foo": "bar",
			},
			expected: Labels{
				"foo": "bar",
			},
		},
		{
			name: "it should keep the original labels when new labels are set",
			target: &metav1.ObjectMeta{
				Labels: Labels{
					"foo": "bar",
				},
			},
			input: Labels{
				"hello": "world",
			},
			expected: Labels{
				"foo":   "bar",
				"hello": "world",
			},
		},
		{
			name: "it should overwrite the existing keys",
			target: &metav1.ObjectMeta{
				Labels: Labels{
					"foo": "bar",
				},
			},
			input: Labels{
				"foo": "baz",
			},
			expected: Labels{
				"foo": "baz",
			},
		},
	}

	for _, tc := range tcs {
		tc.input.OverwriteApplyToObjectMeta(tc.target)
		assert.Equal(t, map[string]string(tc.expected), tc.target.Labels)
	}
}

func TestLabelsMerge(t *testing.T) {
	testLabelsPart1 := Labels{
		"foo":   "bar",
		"hello": "world",
	}
	testLabelsPart2 := Labels{
		"bar":   "foo",
		"hello": "earth",
	}
	expected := map[string]string{
		"foo":   "bar",
		"bar":   "foo",
		"hello": "world",
	}
	assert.Equal(t, expected, map[string]string(testLabelsPart1.Merge(testLabelsPart2)))

	// Test that nil Labels can still be appended to
	testLabelsPart3 := Labels{
		"hello": "world",
	}
	var empty Labels
	assert.Equal(t, map[string]string(testLabelsPart3), map[string]string(empty.Merge(testLabelsPart3)))
}

func TestToValidDNSLabel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"single dash", "-", ""},
		{"multiple dashes", "----", ""},
		{"lc a", "a", "a"},
		{"lc z", "z", "z"},
		{"lc alphabet", "abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxyz"},
		{"UC A", "A", "a"},
		{"UC Z", "Z", "z"},
		{"UC ALPHABET", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz"},
		{"mixed case AlPhAbEt", "AbCdEfGhIjKlMnOpQrStUvWxYz", "abcdefghijklmnopqrstuvwxyz"},
		{"single 0", "0", "d0"},
		{"single 9", "9", "d9"},
		{"single 1", "1", "d1"},
		{"numbers", "01234567890", "d01234567890"},
		{"letters with numbers", "1a0b1c2d3e4f5g6h7i8j9k0", "d1a0b1c2d3e4f5g6h7i8j9k0"},
		{"single / symbol", "/", ""},
		{"single : symbol", ":", ""},
		{"single . symbol", ".", ""},
		{"bunch of symbols", "`~!@#$%^&*()_+-={}[]\\|;':\",.<>/?", ""},
		{"alphabet with symbols",
			"a~b!c@d#e$f^g&h*i(j)k_l-m+n+o[p]q{r}s|t:u;v'w<x,y>z", "a-b-c-d-e-f-g-h-i-j-k-l-m-n-o-p-q-r-s-t-u-v-w-x-y-z"},
		{"multiple symbols between letters", "a//b//c", "a-b-c"},
		{"symbol before", "/a/b/c", "a-b-c"},
		{"symbol after", "a/b/c/", "a-b-c"},
		{"symbols before and after", "/a/b/c/", "a-b-c"},
		{"multiple symbols before after between", "//a//b//c//", "a-b-c"},
		{"mix of all tests except length", "//1a//B-c/d_f/../00-thing.ini/", "d1a-b-c-d-f-00-thing-ini"},
		{"too long input -> middle trim",
			"qwertyuiopqwertyuiopqwertyuiopaaqwertyuiopqwertyuiopqwertyuiopaa",
			"qwertyuiopqwertyuiopqwertyuiop--wertyuiopqwertyuiopqwertyuiopaa"},
		{"too long input but symbols allow for no middle trim",
			"/qwertyuiopqwerty/uiopqwertyuiop//qwertyuiopqwerty/uiopqwertyuiop/",
			"qwertyuiopqwerty-uiopqwertyuiop-qwertyuiopqwerty-uiopqwertyuiop"},
		{"max allowed length but starts with number -> middle trim",
			"123qwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiop",
			"d123qwertyuiopqwertyuiopqwerty--pqwertyuiopqwertyuiopqwertyuiop"},
		{"max allowed length ok",
			"qwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiop123",
			"qwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiopqwertyuiop123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ToValidDNSLabel(tt.input))
		})
	}
}

func Test_cutMiddle(t *testing.T) {
	// not an exported function, so don't bother with extreme cases like 0, 1, 2, or 3 len inputs
	t.Run("len 8 -> 6", func(t *testing.T) {
		assert.Equal(t, "ab--gh", cutMiddle("abcdefgh", 6))
	})
	t.Run("len 9 -> 6", func(t *testing.T) {
		assert.Equal(t, "ab--hi", cutMiddle("abcdefghi", 6))
	})
	t.Run("len 9 -> 7", func(t *testing.T) {
		assert.Equal(t, "ab--ghi", cutMiddle("abcdefghi", 7))
	})
	t.Run("len 10 -> 10", func(t *testing.T) {
		assert.Equal(t, "qwertyuiop", cutMiddle("qwertyuiop", 10))
	})
	// below is what we really want to test
	t.Run("len 63 -> 63", func(t *testing.T) {
		assert.Equal(t,
			"qwertyuiopqwertyuiopqwertyuiop12qwertyuiopqwertyuiopqwertyuiop1",
			cutMiddle("qwertyuiopqwertyuiopqwertyuiop12qwertyuiopqwertyuiopqwertyuiop1", 63))
	})
	t.Run("len 64 -> 63", func(t *testing.T) {
		assert.Equal(t,
			"qwertyuiopqwertyuiopqwertyuiop--wertyuiopqwertyuiopqwertyuiop12",
			cutMiddle("qwertyuiopqwertyuiopqwertyuiop12qwertyuiopqwertyuiopqwertyuiop12", 63))
	})
}
