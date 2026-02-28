package ttlv

import "github.com/ansel1/merry"

// Details prints details from the error, including a stacktrace when available.
func Details(err error) string {
	return merry.Details(err)
}
