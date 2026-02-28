package admin

import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
)

const (
	queryAdminPath = "/admin"
)

func buildQueryPath(endpoint, path, args string) string {
	// Sometimes the API requires single URL key with no values
	// For instance, the Quota code uses the admin API path to "/user?quota"
	// This is done this way since url.Values does not support adding keys without values.
	//
	// So Quota code passes the begining of the query (indicated with a marker "?") in its path already, so we need to escape it
	// and add a separator key instead
	// So we can get something like "/admin/user?quota&" instead of passing two beginning query markers ("?")
	if strings.Contains(path, "?") {
		return fmt.Sprintf("%s%s%s&%s", endpoint, queryAdminPath, path, args)
	}

	return fmt.Sprintf("%s%s%s?%s", endpoint, queryAdminPath, path, args)
}

// valueToURLParams encodes structs into URL query parameters.
func valueToURLParams(i interface{}, accpetableFields []string) url.Values {
	values := url.Values{}

	// Always return json
	values.Add("format", "json")

	getReflect(i, accpetableFields, &values)
	return values
}

func addToURLParams(v *url.Values, i interface{}, acceptableFields []string) {
	getReflect(i, acceptableFields, v)
}

// NOTE: we use linear search here, as none of the API endpoints
// supports more than 10 parameters, in this case asymptotics don't
// matter and we are likely faster this way (even when compared to a
// map).
func contains(tagList []string, tag string) bool {
	for _, tag2 := range tagList {
		if tag == tag2 {
			return true
		}
	}
	return false
}

func getReflect(i interface{}, acceptableFields []string, values *url.Values) {
	t := reflect.TypeOf(i)
	v := reflect.ValueOf(i)

	for b := 0; b < v.NumField(); b++ {
		v2 := v.Field(b)
		tag := t.Field(b).Tag.Get("url")
		if tag == "-" {
			continue
		}
		tagList := strings.Split(tag, ",")
		name := tagList[0]
		if len(name) == 0 {
			name = t.Field(b).Name
		}

		if v2.Kind() == reflect.Struct {
			getReflect(v2.Interface(), acceptableFields, values)
			continue
		}

		if v2.Kind() == reflect.Slice {
			for i := 0; i < v2.Len(); i++ {
				item := v2.Index(i)
				getReflect(item.Interface(), acceptableFields, values)
			}
			continue
		}

		if v2.Kind() == reflect.String ||
			v2.Kind() == reflect.Bool ||
			v2.Kind() == reflect.Int {

			_v2 := fmt.Sprint(v2)
			if len(_v2) > 0 && contains(acceptableFields, name) {
				values.Add(name, _v2)
			}
			continue
		}

		if v2.Kind() == reflect.Ptr && v2.IsValid() && !v2.IsNil() {
			_v2 := fmt.Sprint(v2.Elem())
			if len(_v2) > 0 && contains(acceptableFields, name) {
				values.Add(name, _v2)
			}
			continue
		}
	}
}
