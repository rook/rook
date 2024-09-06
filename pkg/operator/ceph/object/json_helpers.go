package object

import (
	"encoding/json"
	"fmt"
	"strings"
)

// getObjProperty - helper function to manipulate JSON Objects.
// returns nested property of json object.
// Example:
//
//	obj = {"a":{"b":"foo"}}
//	// will return "foo"
//	getObjProperty(obj,"a","b")
func getObjProperty[T string | map[string]interface{} | []interface{}](obj map[string]interface{}, path ...string) (T, error) {
	var res T
	if len(path) == 0 {
		return res, fmt.Errorf("json property path is empty")
	}

	for i, p := range path {
		val, ok := obj[p]
		if !ok {
			return res, fmt.Errorf("json property %q not found", strings.Join(path[:i+1], "."))
		}
		last := i == len(path)-1
		if last {
			// last path segment: get result
			res, ok = val.(T)
			if !ok {
				return res, fmt.Errorf("json property %q is not a %T, got %+v", strings.Join(path, "."), res, val)
			}
			return res, nil
		}
		// walk to the next obj in the path
		obj, ok = val.(map[string]interface{})
		if !ok {
			return res, fmt.Errorf("json property %q is not an object, got %+v", strings.Join(path[:i+1], "."), val)
		}
	}
	// not reachable
	return res, fmt.Errorf("json property %q not found", strings.Join(path, "."))
}

// setObjProperty - helper function to manipulate JSON Objects.
// sets value to json object nested field and returns previous value if presented.
// Example:
//
//	obj = {"a":{"b":"foo"}}
//	// will replace "foo" with "bar" and return "foo"
//	setObjProperty(obj,"bar","a","b")
func setObjProperty[T string | []string | map[string]interface{} | []interface{}](obj map[string]interface{}, val T, path ...string) (T, error) {
	var prev T
	if len(path) == 0 {
		return prev, fmt.Errorf("json property path is empty")
	}
	for i, p := range path {
		last := i == len(path)-1
		if last {
			// last path segment: set result and return prev value
			prevVal, ok := obj[p]
			if ok {
				prevRes, ok := prevVal.(T)
				if ok {
					prev = prevRes
				} else {
					// in go json all arrays are []interface{}, extra conversion for typed arrays (e.g. []string) needed:
					p := new(T)
					if castJson(prevVal, p) {
						prev = *p
					}
				}
			}
			obj[p] = val
			return prev, nil
		}
		// walk to the next obj in the path
		next, ok := obj[p]
		if !ok {
			return prev, fmt.Errorf("json property %q is not found", strings.Join(path[:i+1], "."))
		}
		obj, ok = next.(map[string]interface{})
		if !ok {
			return prev, fmt.Errorf("json property %q is not an object, got %+v", strings.Join(path[:i+1], "."), next)
		}
	}
	// not reachable
	return prev, fmt.Errorf("json property %q not found", strings.Join(path, "."))
}

// castJson - helper function to manipulate JSON Objects.
// Tries to cast any type to any type by converting to JSON and back.
// Returns true on success.
func castJson(in, out interface{}) bool {
	bytes, err := json.Marshal(in)
	if err != nil {
		return false
	}
	err = json.Unmarshal(bytes, out)
	return err == nil
}

// toObj - helper function to manipulate JSON Objects.
// Casts any go struct to map representing JSON object.
func toObj(val interface{}) (map[string]interface{}, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}
	obj := map[string]interface{}{}
	return obj, json.Unmarshal(bytes, &obj)
}

// deepCopyJson - helper function to manipulate JSON Objects.
// Makes deep copy of json object by converting to JSON and back.
func deepCopyJson(in map[string]interface{}) (map[string]interface{}, error) {
	bytes, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	res := map[string]interface{}{}
	err = json.Unmarshal(bytes, &res)
	return res, err
}
