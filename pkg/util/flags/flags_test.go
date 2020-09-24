/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestStringFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Creates a test arg",
	}

	var arg1 string
	var arg2 string
	cmd.Flags().StringVar(&arg1, "foo", "", "test 1")
	cmd.Flags().StringVar(&arg2, "bar", "", "test 2")

	// both arguments are missing
	err := VerifyRequiredFlags(cmd, []string{"foo", "bar"})
	assert.Equal(t, "foo,bar are required for test", err.Error())

	// one argument is missing
	err = cmd.Flags().Set("foo", "fooval")
	assert.NoError(t, err)
	err = VerifyRequiredFlags(cmd, []string{"foo", "bar"})
	assert.Equal(t, "bar is required for test", err.Error())

	// no arguments are missing
	err = cmd.Flags().Set("bar", "barval")
	assert.NoError(t, err)
	err = VerifyRequiredFlags(cmd, []string{"foo", "bar"})
	assert.Nil(t, err)
}

func TestGetFlagsAndValues(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Creates a test arg",
	}

	var arg1 string
	var arg2 string
	cmd.Flags().StringVar(&arg1, "foo-data", "", "test 1")
	cmd.Flags().StringVar(&arg2, "bar-secret", "", "test 2")

	err := cmd.Flags().Set("foo-data", "1234")
	assert.NoError(t, err)
	err = cmd.Flags().Set("bar-secret", "mypassword")
	assert.NoError(t, err)

	// get all flags and their values, providing no filter.  all of them should be returned.
	flagValues := GetFlagsAndValues(cmd.Flags(), "")
	assert.Equal(t, 2, len(flagValues))
	assert.Contains(t, flagValues, "--foo-data=1234")
	assert.Contains(t, flagValues, "--bar-secret=mypassword")

	// get all flags and their values, filtering any flags with "secret" in their name.
	// the --bar-secret flag should be redacted.
	flagValues = GetFlagsAndValues(cmd.Flags(), "secret")
	assert.Equal(t, 2, len(flagValues))
	assert.Contains(t, flagValues, "--foo-data=1234")
	assert.Contains(t, flagValues, "--bar-secret=*****")
}
