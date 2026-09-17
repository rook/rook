/*
Copyright 2022 The Rook Authors. All rights reserved.

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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
)

/*
Example Lock info:

	{
	  "name": "rook-ceph-nfs",
	  "type": "exclusive",
	  "tag": "rook-ceph-nfs",
	  "lockers": [
	    {
	      "name": "client.28945",
	      "cookie": "test-cookie",
	      "description": "",
	      "expiration": "2022-09-08T23:56:57.924802+0000",
	      "addr": "10.244.0.44:0/960227889"
	    }
	  ]
	}
*/
type radosLockInfo struct {
	Name    string            `json:"name"`
	Tag     string            `json:"tag"`
	Lockers []radosLockerInfo `json:"lockers"`
}

type radosLockerInfo struct {
	Name   string `json:"name"`
	Cookie string `json:"cookie"`
}

// RadosLockObject locks a rados object in a given pool and namespace and returns a lock "cookie"
// that can be used to identify this unique lock.
func RadosLockObject(
	context *clusterd.Context, clusterInfo *ClusterInfo,
	pool, namespace, objectName, lockName string,
	lockTimeout time.Duration,
) (string, error) {
	// generate a random "cookie" that identifies this lock "session"
	cookieLen := 12
	b := make([]byte, cookieLen)
	if _, err := rand.Read(b); err != nil {
		return "", errors.Wrapf(err, "failed to generate cookie for lock %q on rados object rados://%s/%s/%s",
			lockName, pool, namespace, objectName)
	}
	cookie := fmt.Sprintf("%x", b)[:cookieLen]

	cmd := NewRadosCommand(context, clusterInfo, []string{
		"--pool", pool,
		"--namespace", namespace,
		"lock", "get", objectName, lockName,
		"--lock-tag", lockName, // assume we aren't making many locks; use lock name as the tag
		"--lock-cookie", cookie,
		"--lock-duration", fmt.Sprintf("%d", int(math.Ceil(lockTimeout.Seconds()))),
	})
	if _, err := cmd.RunWithTimeout(exec.CephCommandsTimeout); err != nil {
		return "", errors.Wrapf(err, "failed to acquire lock %q on rados object rados://%s/%s/%s",
			lockName, pool, namespace, objectName)
	}

	return cookie, nil
}

// RadosUnlockObject unlocks a rados object in a given pool and namespace by searching for locks on
// the object that match the "cookie" obtained from a RadosLockObject() call.
func RadosUnlockObject(
	context *clusterd.Context, clusterInfo *ClusterInfo,
	pool, namespace, objectName, lockName string,
	lockCookie string,
) error {
	lockInfo, err := radosObjectLockInfo(context, clusterInfo, pool, namespace, objectName, lockName, lockCookie)
	if err != nil {
		return errors.Wrap(err, "failed to unlock object")
	}

	if lockInfo.Name != lockName || lockInfo.Tag != lockName {
		// some other lock has locked the object, but that means the lock with the given cookie no
		// longer has it locked; treat this as success
		logger.Infof("rados object rados://%s/%s/%s is not locked by lock %q but is locked by %q",
			pool, namespace, objectName, lockName, lockInfo.Name)
		return nil
	}

	if len(lockInfo.Lockers) == 0 {
		logger.Infof("rados object rados://%s/%s/%s is already fully unlocked (lock: %q, cookie: %q)",
			pool, namespace, objectName, lockName, lockCookie)
		return nil
	}

	locker := findLockerWithCookie(lockInfo.Lockers, lockCookie)
	if locker == nil {
		// this cookie is not locking the object but another is; still treat this as success but log it
		logger.Infof("rados object rados://%s/%s/%s is not locked by lock %q with cookie %q but is locked: %+v",
			pool, namespace, objectName, lockName, lockCookie, lockInfo.Lockers)
		return nil
	}

	cmd := NewRadosCommand(context, clusterInfo, []string{
		"--pool", pool,
		"--namespace", namespace,
		"lock", "break", objectName, lockName, locker.Name, // breaking the lock also requires locker name
		"--lock-tag", lockName, // assume we aren't making many locks; use lock name as the tag
		"--lock-cookie", lockCookie,
	})
	if _, err := cmd.RunWithTimeout(exec.CephCommandsTimeout); err != nil {
		return errors.Wrapf(err, "failed to unlock lock %q on rados object rados://%s/%s/%s",
			lockName, pool, namespace, objectName)
	}

	return nil
}

func radosObjectLockInfo(
	context *clusterd.Context, clusterInfo *ClusterInfo,
	pool, namespace, objectName, lockName string,
	lockCookie string,
) (radosLockInfo, error) {
	cmd := NewRadosCommand(context, clusterInfo, []string{
		"--pool", pool,
		"--namespace", namespace,
		"lock", "info", objectName, lockName,
		"--format", "json",
	})
	cmd.JsonOutput = true
	rawInfo, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return radosLockInfo{}, errors.Wrapf(err, "failed to get lock %q info for rados object rados://%s/%s/%s",
			lockName, pool, namespace, objectName)
	}

	lockInfo := radosLockInfo{}
	if err := json.Unmarshal(rawInfo, &lockInfo); err != nil {
		return radosLockInfo{}, errors.Wrapf(err, "failed to parse lock %q info for rados object rados://%s/%s/%s",
			lockName, pool, namespace, objectName)
	}

	return lockInfo, nil
}

func findLockerWithCookie(lockers []radosLockerInfo, cookie string) *radosLockerInfo {
	for _, locker := range lockers {
		if locker.Cookie == cookie {
			return &locker
		}
	}
	return nil
}

// RadosRemoveObject idempotently removes a rados object from the given pool and namespace.
func RadosRemoveObject(
	context *clusterd.Context, clusterInfo *ClusterInfo,
	pool, namespace, objectName string,
) error {
	cmd := NewRadosCommand(context, clusterInfo, []string{
		"--pool", pool,
		"--namespace", namespace,
		"stat", objectName,
	})
	_, err := cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		if exec.IsTimeout(err) {
			return errors.Wrapf(err, "failed to determine if rados object rados://%s/%s/%s exists before removing it",
				pool, namespace, objectName)
		}
		// assume any other error means the object already doesn't exist
		logger.Debugf("rados object rados:/%s/%s/%s being removed is assumed to not exist after stat-ing: %v",
			pool, namespace, objectName, err)
		return nil
	}

	cmd = NewRadosCommand(context, clusterInfo, []string{
		"--pool", pool,
		"--namespace", namespace,
		"rm", objectName,
	})
	_, err = cmd.RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to remove rados object rados://%s/%s/%s", pool, namespace, objectName)
	}

	return nil
}
