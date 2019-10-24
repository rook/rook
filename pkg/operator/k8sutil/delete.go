/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package k8sutil

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseKubernetesDeleteOptions returns the base set of Kubernetes delete options which most delete
// operations should use.
func BaseKubernetesDeleteOptions() *metav1.DeleteOptions {
	p := metav1.DeletePropagationForeground
	g := int64(0)
	return &metav1.DeleteOptions{GracePeriodSeconds: &g, PropagationPolicy: &p}
}

// DeleteOptions are a common set of options controlling the behavior of k8sutil delete operations.
// DeleteOptions is a superset of WaitOptions.
type DeleteOptions struct {
	// MustDelete controls the idempotency of the delete operation. If MustDelete is true and the
	// resource being deleted does not exist, the delete operation is considered a failure. If
	// MustDelete is false and the resource being deleted does not exist, the delete operation is
	// considered a success.
	MustDelete bool

	// DeleteOptions is a superset of WaitOptions.
	WaitOptions
}

// Use this for unit testing
var unitTestRetryIntervalRecord time.Duration

// DeleteResource implements the DeleteOptions logic around deletion of a Kubernetes resource.
//
// The delete and verify functions used as parameters should implement and return the error from the
// Kubernetes `Delete` and `Get` commands respectively.
//
// The resource string will be used to report the resource in log and error messages. A good pattern
// would be to set this string to the resource type (e.g., Deployment) and the name of the resource
// (e.g., my-deployment).
//
// The default wait options should specify sane defaults for retry count and retry interval for
// deletion of the specific resource type. Only retry count and interval will be used from this
// parameter.
func DeleteResource(
	delete func() error,
	verify func() error,
	resource string,
	opts *DeleteOptions,
	defaultWaitOptions *WaitOptions,
) error {
	if err := delete(); err != nil {
		if errors.IsNotFound(err) && opts.MustDelete {
			return fmt.Errorf("failed to delete %s; it does not exist. %+v", resource, err)
		} else if errors.IsNotFound(err) && !opts.MustDelete {
			logger.Debugf("%s is already deleted", resource)
			return nil
		}
		return fmt.Errorf("failed to delete %s. %+v", resource, err)
	}

	if !opts.Wait {
		return nil
	}

	// if default wait options aren't set, still provide something of a default
	retries := opts.retryCountOrDefault(defaultWaitOptions.retryCountOrDefault(30))
	retryInterval := opts.retryIntervalOrDefault(defaultWaitOptions.retryIntervalOrDefault(5 * time.Second))
	unitTestRetryIntervalRecord = retryInterval
	var i uint
	var err error
	// less-than-or-equal is important; i=0 is the first "try". It isn't a "re-try" until i=1.
	for i = 0; i <= retries; i++ {
		err = verify()
		if err != nil && errors.IsNotFound(err) {
			logger.Debugf("%s was deleted after %d retries every %v seconds", resource, i, retryInterval/time.Second)
			return nil
		}
		logger.Infof("Retrying %d more times every %d seconds for %s to be deleted",
			retries-i, int(retryInterval/time.Second), resource)
		if retries-i > 0 {
			time.Sleep(retryInterval)
		}
	}

	msg := fmt.Sprintf("failed to delete %s. gave up waiting after %d retries every %v seconds. %+v",
		resource, retries, retryInterval/time.Second, err)
	if opts.ErrorOnTimeout {
		return fmt.Errorf(msg)
	}
	logger.Warningf(msg)
	return nil
}
