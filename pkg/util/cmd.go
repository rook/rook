package util

import (
	"strings"
)

func SplitList(list string) []string {
	if list == "" {
		return nil
	}

	return strings.Split(list, ",")
}
