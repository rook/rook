package kmip

import (
	"errors"
	"fmt"

	"github.com/ansel1/merry"
	"github.com/gemalto/kmip-go/kmip14"
)

func Details(err error) string {
	return merry.Details(err)
}

var ErrInvalidTag = errors.New("invalid tag")

type errKey int

const (
	errorKeyResultReason errKey = iota
)

//nolint:gochecknoinits
func init() {
	merry.RegisterDetail("Result Reason", errorKeyResultReason)
}

func WithResultReason(err error, rr kmip14.ResultReason) error {
	return merry.WithValue(err, errorKeyResultReason, rr)
}

func GetResultReason(err error) kmip14.ResultReason {
	v := merry.Value(err, errorKeyResultReason)
	switch t := v.(type) {
	case nil:
		return kmip14.ResultReason(0)
	case kmip14.ResultReason:
		return t
	default:
		panic(fmt.Sprintf("err result reason attribute's value was wrong type, expected ResultReason, got %T", v))
	}
}
