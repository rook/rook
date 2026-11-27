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
package client

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateProfile(t *testing.T) {
	testCreateProfile(t, "", "myroot", "", 0, 0)
}

func TestCreateProfileWithFailureDomain(t *testing.T) {
	testCreateProfile(t, "osd", "", "", 0, 0)
}

func TestCreateProfileWithDeviceClass(t *testing.T) {
	testCreateProfile(t, "osd", "", "hdd", 0, 0)
}

func TestCreateProfileWithMSR(t *testing.T) {
	testCreateProfile(t, "host", "", "", 5, 3)
}

func testCreateProfile(t *testing.T, failureDomain, crushRoot, deviceClass string, failureDomains, osdsPerDomain int32) {
	stripeUnit := resource.MustParse("4Ki")
	spec := cephv1.PoolSpec{
		FailureDomain: failureDomain,
		CrushRoot:     crushRoot,
		DeviceClass:   deviceClass,
		ErasureCoded: cephv1.ErasureCodedSpec{
			DataChunks:                2,
			CodingChunks:              3,
			StripeUnit:                &stripeUnit,
			CrushNumFailureDomains:    failureDomains,
			CrushOSDsPerFailureDomain: osdsPerDomain,
		},
	}

	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "erasure-code-profile" {
			if args[2] == "get" {
				assert.Equal(t, "default", args[3])
				return `{"plugin":"myplugin","technique":"t"}`, nil
			}
			if args[2] == "set" {
				assert.Equal(t, "myapp", args[3])
				assert.Equal(t, "--force", args[4])
				assert.Equal(t, fmt.Sprintf("k=%d", spec.ErasureCoded.DataChunks), args[5])
				assert.Equal(t, fmt.Sprintf("m=%d", spec.ErasureCoded.CodingChunks), args[6])
				assert.Equal(t, "plugin=myplugin", args[7])
				assert.Equal(t, "technique=t", args[8])
				nextArg := 9
				if failureDomain != "" {
					assert.Equal(t, fmt.Sprintf("crush-failure-domain=%s", failureDomain), args[nextArg])
					nextArg++
				}
				if crushRoot != "" {
					assert.Equal(t, fmt.Sprintf("crush-root=%s", crushRoot), args[nextArg])
					nextArg++
				}
				if deviceClass != "" {
					assert.Equal(t, fmt.Sprintf("crush-device-class=%s", deviceClass), args[nextArg])
					nextArg++
				}
				if failureDomains > 0 {
					assert.Equal(t, fmt.Sprintf("crush-num-failure-domains=%d", failureDomains), args[nextArg])
					nextArg++
				}
				if osdsPerDomain > 0 {
					assert.Equal(t, fmt.Sprintf("crush-osds-per-failure-domain=%d", osdsPerDomain), args[nextArg])
					nextArg++
				}
				if spec.ErasureCoded.StripeUnit != nil && !spec.ErasureCoded.StripeUnit.IsZero() {
					stripeBytes, ok := spec.ErasureCoded.StripeUnit.AsInt64()
					assert.True(t, ok)
					assert.Equal(t, fmt.Sprintf("stripe_unit=%d", stripeBytes), args[nextArg])
					assert.Equal(t, stripeBytes, int64(4096), "Validate the resource conversion to bytes")
				}
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := CreateErasureCodeProfile(context, AdminTestClusterInfo("mycluster"), "myapp", spec)
	assert.Nil(t, err)
}

func TestCreateProfileTechnique(t *testing.T) {
	var plugin, technique string
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "erasure-code-profile" {
			if args[2] == "get" {
				assert.Equal(t, "default", args[3])
				return `{"plugin":"myplugin","technique":"t"}`, nil
			}
			if args[2] == "set" {
				assert.Equal(t, "myapp", args[3])
				assert.Equal(t, "k=8", args[5])
				assert.Equal(t, "m=6", args[6])
				plugin = args[7]
				technique = args[8]
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	createProfile := func(algorithm, ecTechnique string) {
		plugin, technique = "", ""
		spec := cephv1.PoolSpec{
			ErasureCoded: cephv1.ErasureCodedSpec{
				DataChunks:   8,
				CodingChunks: 6,
				Algorithm:    algorithm,
				Technique:    ecTechnique,
			},
		}
		err := CreateErasureCodeProfile(context, AdminTestClusterInfo("mycluster"), "myapp", spec)
		assert.NoError(t, err)
	}

	t.Run("technique from the default profile when not set", func(t *testing.T) {
		createProfile("", "")
		assert.Equal(t, "plugin=myplugin", plugin)
		assert.Equal(t, "technique=t", technique)
	})

	t.Run("technique set", func(t *testing.T) {
		createProfile("", "cauchy")
		assert.Equal(t, "plugin=myplugin", plugin)
		assert.Equal(t, "technique=cauchy", technique)
	})

	t.Run("algorithm set without technique", func(t *testing.T) {
		createProfile("isa", "")
		assert.Equal(t, "plugin=isa", plugin)
		assert.Equal(t, "technique=t", technique)
	})

	t.Run("algorithm and technique set", func(t *testing.T) {
		createProfile("isa", "cauchy")
		assert.Equal(t, "plugin=isa", plugin)
		assert.Equal(t, "technique=cauchy", technique)
	})
}

// Rook sets the profile of an existing pool on every reconcile. Ceph refuses to change a profile
// that differs from the existing one unless --yes-i-really-mean-it is passed as well as --force, so
// changing the technique of an existing pool fails the reconcile instead of rewriting its profile.
func TestCreateProfileChangedOnExistingPool(t *testing.T) {
	var setArgs []string
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if args[1] == "erasure-code-profile" {
			if args[2] == "get" {
				return `{"plugin":"isa","technique":"reed_sol_van"}`, nil
			}
			if args[2] == "set" {
				setArgs = args
				return "", errors.New("Error EPERM: overriding erasure code profile can be DANGEROUS; add --yes-i-really-mean-it to do it anyway")
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	spec := cephv1.PoolSpec{ErasureCoded: cephv1.ErasureCodedSpec{DataChunks: 2, CodingChunks: 1, Technique: "cauchy"}}
	err := CreateErasureCodeProfile(context, AdminTestClusterInfo("mycluster"), "mypool_ecprofile", spec)
	assert.ErrorContains(t, err, "failed to set ec-profile")
	assert.Contains(t, setArgs, "--force")
	assert.NotContains(t, setArgs, "--yes-i-really-mean-it")
}
