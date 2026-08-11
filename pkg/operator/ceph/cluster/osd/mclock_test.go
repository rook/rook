/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package osd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const mclockTestNamespace = "rook-ceph"

func resetMclockCapacityUpdated() {
	mclockCapacityUpdated = sync.Map{}
}

func mclockDone(osdID int) bool {
	_, ok := mclockCapacityUpdated.Load(mclockTestNamespace + "/" + fmt.Sprint(osdID))
	return ok
}

func newMclockCluster(executor *exectest.MockExecutor) *Cluster {
	clusterInfo := client.AdminTestClusterInfo(mclockTestNamespace)
	clusterInfo.Context = context.TODO()
	return &Cluster{
		context:     &clusterd.Context{Executor: executor},
		clusterInfo: clusterInfo,
	}
}

func mclockExecutor(mock func(args ...string) (string, error)) *exectest.MockExecutor {
	run := func(command string, args ...string) (string, error) {
		return mock(args...)
	}
	return &exectest.MockExecutor{
		MockExecuteCommandWithOutput: run,
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			return run(command, args...)
		},
	}
}

func TestOsdIDIsUp(t *testing.T) {
	id, ok := osdIDIsUp("1", "2")
	assert.True(t, ok)
	assert.Equal(t, 2, id)

	id, ok = osdIDIsUp("0", "0")
	assert.False(t, ok)
	assert.Equal(t, 0, id)

	id, ok = osdIDIsUp("x", "0")
	assert.False(t, ok)

	id, ok = osdIDIsUp("1", "x")
	assert.False(t, ok)
}

func TestUpdateMclockCapacity(t *testing.T) {
	key := mclockTestNamespace + "/0"
	monStore := func(c *Cluster) *opconfig.MonStore {
		return opconfig.GetMonStore(c.context, c.clusterInfo)
	}

	t.Run("sets hdd capacity when bench is higher", func(t *testing.T) {
		resetMclockCapacityUpdated()
		var setOption, setValue string
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_type":"hdd","bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":8735.34}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				setOption, setValue = args[3], args[4]
				return "", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))

		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.NoError(t, err)
		assert.Equal(t, mclockIopsHDD, setOption)
		assert.Equal(t, "8735", setValue)
		assert.True(t, mclockDone(0))
	})

	t.Run("sets ssd capacity for non-rotational device", func(t *testing.T) {
		resetMclockCapacityUpdated()
		var setOption string
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 3 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_type":"ssd","bluestore_bdev_rotational":"0"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "1000", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":21500.6}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				setOption = args[3]
				return "", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))

		err := c.updateMclockCapacity(1, monStore(c), mclockTestNamespace+"/1")
		assert.NoError(t, err)
		assert.Equal(t, mclockIopsSSD, setOption)
		assert.True(t, mclockDone(1))
	})

	t.Run("metadata lookup fails", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "osd" && args[1] == "metadata" {
				return "", fmt.Errorf("metadata failed")
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.Error(t, err)
		assert.False(t, mclockDone(0))
	})

	t.Run("config get fails", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "", fmt.Errorf("config get failed")
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.ErrorContains(t, err, "failed to get current iops")
		assert.False(t, mclockDone(0))
	})

	t.Run("current iops is not numeric", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "not-a-number", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.ErrorContains(t, err, "failed to parse current iops")
		assert.False(t, mclockDone(0))
	})

	t.Run("cache drop fails", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", fmt.Errorf("cache drop failed")
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.ErrorContains(t, err, "cache drop")
		assert.False(t, mclockDone(0))
	})

	t.Run("bench fails", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return "", fmt.Errorf("bench failed")
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.ErrorContains(t, err, "bench")
		assert.False(t, mclockDone(0))
	})

	t.Run("bench iops is not numeric", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":"bad"}`, nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.ErrorContains(t, err, "bench result")
		assert.False(t, mclockDone(0))
	})

	t.Run("config set fails", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":8735}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				return "", fmt.Errorf("config set failed")
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.Error(t, err)
		assert.False(t, mclockDone(0))
	})

	t.Run("bench lower than current skips config set", func(t *testing.T) {
		resetMclockCapacityUpdated()
		configSet := false
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "9000", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":8000}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				configSet = true
				return "", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.NoError(t, err)
		assert.False(t, configSet)
		assert.True(t, mclockDone(0))
	})

	t.Run("bench equal to current skips config set", func(t *testing.T) {
		resetMclockCapacityUpdated()
		configSet := false
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "8735", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":8735}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				configSet = true
				return "", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		err := c.updateMclockCapacity(0, monStore(c), key)
		assert.NoError(t, err)
		assert.False(t, configSet)
		assert.True(t, mclockDone(0))
	})
}

func TestEnsureMclockCapacityForOSDs(t *testing.T) {
	t.Run("sets capacity for up osds", func(t *testing.T) {
		resetMclockCapacityUpdated()
		var setValue string
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "dump":
				return `{"osds":[{"osd":"0","up":"1","in":"1"}]}`, nil
			case len(args) >= 2 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":8735}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				setValue = args[4]
				return "", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		c.ensureMclockCapacityForOSDs()
		assert.Equal(t, "8735", setValue)
		assert.True(t, mclockDone(0))
	})

	t.Run("disabled by operator setting", func(t *testing.T) {
		resetMclockCapacityUpdated()
		t.Setenv(disableOSDBenchmarkEnv, "true")
		defer os.Unsetenv(disableOSDBenchmarkEnv)

		benchCalls := 0
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			if len(args) >= 3 && args[0] == "tell" && args[2] == "bench" {
				benchCalls++
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		c.ensureMclockCapacityForOSDs()
		assert.Equal(t, 0, benchCalls)
	})

	t.Run("osd dump failure is a no-op", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			if len(args) >= 2 && args[0] == "osd" && args[1] == "dump" {
				return "", fmt.Errorf("dump failed")
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		c.ensureMclockCapacityForOSDs()
		assert.False(t, mclockDone(0))
	})

	t.Run("skips osd that is not up", func(t *testing.T) {
		resetMclockCapacityUpdated()
		benchCalls := 0
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "dump":
				return `{"osds":[{"osd":"0","up":"0","in":"1"}]}`, nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				benchCalls++
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		c.ensureMclockCapacityForOSDs()
		assert.Equal(t, 0, benchCalls)
	})

	t.Run("does not re-bench osd already marked done", func(t *testing.T) {
		resetMclockCapacityUpdated()
		mclockCapacityUpdated.Store(mclockTestNamespace+"/0", struct{}{})
		benchCalls := 0
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "dump":
				return `{"osds":[{"osd":"0","up":"1","in":"1"}]}`, nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				benchCalls++
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		c.ensureMclockCapacityForOSDs()
		assert.Equal(t, 0, benchCalls)
	})

	t.Run("continues when one osd fails", func(t *testing.T) {
		resetMclockCapacityUpdated()
		c := newMclockCluster(mclockExecutor(func(args ...string) (string, error) {
			switch {
			case len(args) >= 2 && args[0] == "osd" && args[1] == "dump":
				return `{"osds":[{"osd":"0","up":"1","in":"1"},{"osd":"1","up":"1","in":"1"}]}`, nil
			case len(args) >= 3 && args[0] == "osd" && args[1] == "metadata" && args[2] == "0":
				return "", fmt.Errorf("metadata failed for osd.0")
			case len(args) >= 3 && args[0] == "osd" && args[1] == "metadata":
				return `{"bluestore_bdev_rotational":"1"}`, nil
			case len(args) >= 4 && args[0] == "config" && args[1] == "get":
				return "315", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "cache":
				return "", nil
			case len(args) >= 3 && args[0] == "tell" && args[2] == "bench":
				return `{"iops":8735}`, nil
			case len(args) >= 5 && args[0] == "config" && args[1] == "set":
				return "", nil
			}
			return "", fmt.Errorf("unexpected args %v", args)
		}))
		c.ensureMclockCapacityForOSDs()
		assert.False(t, mclockDone(0))
		assert.True(t, mclockDone(1))
	})
}
