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
	cmd.Flags().Set("foo", "fooval")
	err = VerifyRequiredFlags(cmd, []string{"foo", "bar"})
	assert.Equal(t, "bar is required for test", err.Error())

	// no arguments are missing
	cmd.Flags().Set("bar", "barval")
	err = VerifyRequiredFlags(cmd, []string{"foo", "bar"})
	assert.Nil(t, err)
}

func TestUintFlags(t *testing.T) {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Creates a test arg",
	}

	var arg1 uint64
	var arg2 uint64
	cmd.Flags().Uint64Var(&arg1, "foo", 0, "test 1")
	cmd.Flags().Uint64Var(&arg2, "bar", 0, "test 2")

	// both arguments are missing
	err := VerifyRequiredUint64Flags(cmd, []string{"foo", "bar"})
	assert.Equal(t, "foo,bar are required for test", err.Error())

	// one argument is missing
	cmd.Flags().Set("foo", "1234")
	err = VerifyRequiredUint64Flags(cmd, []string{"foo", "bar"})
	assert.Equal(t, "bar is required for test", err.Error())

	// no arguments are missing
	cmd.Flags().Set("bar", "5432")
	err = VerifyRequiredUint64Flags(cmd, []string{"foo", "bar"})
	assert.Nil(t, err)
}
