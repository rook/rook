package bucket

import (
	"time"

	"github.com/pkg/errors"
	storagev1 "k8s.io/api/storage/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	waitDuration = time.Second * 3
	waitFactor   = 2
	waitJitter   = 0.5
	waitSteps    = 5
	waitCap      = time.Minute * 5
)

var backoff = wait.Backoff{
	Duration: waitDuration,
	Factor:   waitFactor,
	Jitter:   waitJitter,
	Steps:    waitSteps,
	Cap:      waitCap,
}

func (p *Provisioner) getStorageClassWithBackoff(name string) (class *storagev1.StorageClass, err error) {
	logger.Infof("getting storage class %q", name)
	classClient := p.context.Clientset.StorageV1().StorageClasses()
	// Retry Get() with backoff.  Errors other than IsNotFound are ignored.
	err = wait.ExponentialBackoff(backoff, func() (done bool, err error) {
		class, err = classClient.Get(p.clusterInfo.Context, name, metav1.GetOptions{})
		if err == nil {
			return true, nil
		}
		if kerrors.IsNotFound(err) {
			return true, err
		}
		logger.Errorf("error getting class %q, retrying. %v", name, err)
		return false, nil
	})
	if err != nil {
		return nil, errors.Wrapf(err, "unable to Get storageclass %q", name)
	}
	return
}
