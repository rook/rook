package display

import "strconv"

func NumToStrOmitEmpty(num uint) string {
	if num == 0 {
		return ""
	}

	return strconv.FormatUint(uint64(num), 10)
}
