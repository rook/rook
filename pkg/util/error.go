/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package util

import (
	"fmt"

	"github.com/pkg/errors"
)

// AggregateErrors takes a list of errors formats them into a pretty, user-readable list headed by
// the text "errors:". All errors in the list will lose any context besides their error string.
// If the errs list is empty, AggregateErrors returns nil.
// Example:
//
//	AggregateErrors(errList, "errors for my %q", "mom")  -->
//	`errors for my "mom":
//	    error 1
//	    error 2
//	    etc.`
func AggregateErrors(errs []error, format string, args ...interface{}) error {
	if len(errs) == 0 {
		return nil
	}

	errString := fmt.Sprintf(format+":", args...)
	for _, err := range errs {
		errString = fmt.Sprintf("%s\n    %s", errString, err.Error())
	}
	return errors.Errorf(errString)
}
