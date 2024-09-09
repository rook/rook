package object

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getObjPropertyStr(t *testing.T) {
	type args struct {
		json string
		path []string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				json: `{"a":{"b":"val"}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "val",
			wantErr: false,
		},
		{
			name: "success: empty str",
			args: args{
				json: `{"a":{"b":""}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: false,
		},
		{
			name: "err: empty json",
			args: args{
				json: `{}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "err: is obj",
			args: args{
				json: `{"a":{"b":{"val":"val"}}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "err: is arr",
			args: args{
				json: `{"a":{"b":["val1","val2"]}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "err: is bool",
			args: args{
				json: `{"a":{"b":true}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "err: is num",
			args: args{
				json: `{"a":{"b":5}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "err: is missing",
			args: args{
				json: `{"a":{"c":"val"}}`,
				path: []string{
					"a", "b",
				},
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{}
			_ = json.Unmarshal([]byte(tt.args.json), &obj)
			got, err := getObjProperty[string](obj, tt.args.path...)
			if (err != nil) != tt.wantErr {
				t.Errorf("getObjProperty() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getObjProperty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getObjPropertyObjArr(t *testing.T) {
	type args struct {
		json string
		path []string
	}
	tests := []struct {
		name    string
		args    args
		want    []interface{}
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				json: `{"a":{"b":[
          {"c":"val1"},
          {"d":"val2"}
        ]}}`,
				path: []string{
					"a", "b",
				},
			},
			want: []interface{}{
				map[string]interface{}{"c": "val1"},
				map[string]interface{}{"d": "val2"},
			},
			wantErr: false,
		},
		{
			name: "err: empty json",
			args: args{
				json: `{}`,
				path: []string{
					"a", "b",
				},
			},
			wantErr: true,
		},
		{
			name: "err: is obj",
			args: args{
				json: `{"a":{"b":{"val":"val"}}}`,
				path: []string{
					"a", "b",
				},
			},
			wantErr: true,
		},
		{
			name: "err: is bool",
			args: args{
				json: `{"a":{"b":true}}`,
				path: []string{
					"a", "b",
				},
			},
			wantErr: true,
		},
		{
			name: "err: is num",
			args: args{
				json: `{"a":{"b":5}}`,
				path: []string{
					"a", "b",
				},
			},
			wantErr: true,
		},
		{
			name: "err: is missing",
			args: args{
				json: `{"a":{"c":"val"}}`,
				path: []string{
					"a", "b",
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{}
			_ = json.Unmarshal([]byte(tt.args.json), &obj)
			got, err := getObjProperty[[]interface{}](obj, tt.args.path...)
			if (err != nil) != tt.wantErr {
				t.Errorf("getObjProperty() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getObjProperty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_setObjProperty(t *testing.T) {
	type args struct {
		json string
		val  string
		path []string
	}
	tests := []struct {
		name     string
		args     args
		wantPrev string
		wantJSON string
		wantErr  bool
	}{
		{
			name: "replace val",
			args: args{
				json: `{"a":{"b":"val"}}`,
				val:  "new val",
				path: []string{
					"a", "b",
				},
			},
			wantPrev: "val",
			wantJSON: `{"a":{"b":"new val"}}`,
			wantErr:  false,
		},
		{
			name: "same val",
			args: args{
				json: `{"a":{"b":"val"}}`,
				val:  "val",
				path: []string{
					"a", "b",
				},
			},
			wantPrev: "val",
			wantJSON: `{"a":{"b":"val"}}`,
			wantErr:  false,
		},
		{
			name: "add val",
			args: args{
				json: `{"a":{"b":"val"}}`,
				val:  "val2",
				path: []string{
					"a", "c",
				},
			},
			wantPrev: "",
			wantJSON: `{"a":{"b":"val","c":"val2"}}`,
			wantErr:  false,
		},
		{
			name: "add root val",
			args: args{
				json: `{"a":{"b":"val"}}`,
				val:  "val2",
				path: []string{
					"c",
				},
			},
			wantPrev: "",
			wantJSON: `{"a":{"b":"val"},"c":"val2"}`,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.json), &obj)
			assert.NoError(t, err)
			prev, err := setObjProperty(obj, tt.args.val, tt.args.path...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, tt.wantPrev, prev)
			bytes, err := json.Marshal(obj)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(bytes))
		})
	}
}
func Test_setObjPropertyObj(t *testing.T) {
	type args struct {
		json string
		val  map[string]interface{}
		path []string
	}
	tests := []struct {
		name     string
		args     args
		wantPrev map[string]interface{}
		wantJSON string
		wantErr  bool
	}{
		{
			name: "add obj",
			args: args{
				json: `{"a":{"b":{}}}`,
				val:  map[string]interface{}{"c": "val1"},
				path: []string{
					"a", "b",
				},
			},
			wantPrev: map[string]interface{}{},
			wantJSON: `{"a":{"b":{"c":"val1"}}}`,
			wantErr:  false,
		},
		{
			name: "set obj",
			args: args{
				json: `{"a":{"b":{"c": "val1"}}}`,
				val:  map[string]interface{}{"d": "val2"},
				path: []string{
					"a", "b",
				},
			},
			wantPrev: map[string]interface{}{"c": "val1"},
			wantJSON: `{"a":{"b":{"d":"val2"}}}`,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.json), &obj)
			assert.NoError(t, err)
			prev, err := setObjProperty(obj, tt.args.val, tt.args.path...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, tt.wantPrev, prev)
			bytes, err := json.Marshal(obj)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(bytes))
		})
	}
}

func Test_setObjPropertyArr(t *testing.T) {
	type args struct {
		json string
		val  []interface{}
		path []string
	}
	tests := []struct {
		name     string
		args     args
		wantPrev []interface{}
		wantJSON string
		wantErr  bool
	}{
		{
			name: "set obj arr",
			args: args{
				json: `{"a":{"b":{}}}`,
				val: []interface{}{
					map[string]interface{}{"c": "val1"},
					map[string]interface{}{"d": "val2"},
				},
				path: []string{
					"a", "b",
				},
			},
			wantPrev: nil,
			wantJSON: `{"a":{"b":[{"c":"val1"},{"d":"val2"}]}}`,
			wantErr:  false,
		},
		{
			name: "add obj arr",
			args: args{
				json: `{"a":{"b":[{"c": "val"}]}}`,
				val: []interface{}{
					map[string]interface{}{"d": "val1"},
					map[string]interface{}{"e": "val2"},
				},
				path: []string{
					"a", "b",
				},
			},
			wantPrev: []interface{}{
				map[string]interface{}{"c": "val"},
			},
			wantJSON: `{"a":{"b":[{"d":"val1"},{"e":"val2"}]}}`,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.json), &obj)
			assert.NoError(t, err)
			prev, err := setObjProperty(obj, tt.args.val, tt.args.path...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, tt.wantPrev, prev)
			bytes, err := json.Marshal(obj)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(bytes))
		})
	}
}
func Test_setObjPropertyStrArr(t *testing.T) {
	type args struct {
		json string
		val  []string
		path []string
	}
	tests := []struct {
		name     string
		args     args
		wantPrev []string
		wantJSON string
		wantErr  bool
	}{
		{
			name: "add str arr",
			args: args{
				json: `{"a":{"b":{}}}`,
				val:  []string{"c", "d"},
				path: []string{
					"a", "b",
				},
			},
			wantPrev: nil,
			wantJSON: `{"a":{"b":["c","d"]}}`,
			wantErr:  false,
		},
		{
			name: "set str arr",
			args: args{
				json: `{"a":{"b":["val"]}}`,
				val:  []string{"c", "d"},
				path: []string{
					"a", "b",
				},
			},
			wantPrev: []string{"val"},
			wantJSON: `{"a":{"b":["c","d"]}}`,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{}
			err := json.Unmarshal([]byte(tt.args.json), &obj)
			assert.NoError(t, err)
			prev, err := setObjProperty(obj, tt.args.val, tt.args.path...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.EqualValues(t, tt.wantPrev, prev)
			bytes, err := json.Marshal(obj)
			assert.NoError(t, err)
			assert.JSONEq(t, tt.wantJSON, string(bytes))
		})
	}
}

func Test_deepCopyJson(t *testing.T) {
	in := map[string]interface{}{
		"key": []interface{}{"1", "2", "3"},
	}
	inCopy, err := deepCopyJson(in)
	assert.NoError(t, err)
	assert.EqualValues(t, in, inCopy)

	assert.EqualValues(t, []interface{}{"1", "2", "3"}, in["key"])
	assert.EqualValues(t, []interface{}{"1", "2", "3"}, inCopy["key"])

	inCopy["key"].([]interface{})[1] = "7"

	assert.EqualValues(t, []interface{}{"1", "2", "3"}, in["key"])
	assert.EqualValues(t, []interface{}{"1", "7", "3"}, inCopy["key"])
}
