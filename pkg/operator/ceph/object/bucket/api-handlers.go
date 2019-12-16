package bucket

import (
	"time"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
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
		class, err = classClient.Get(name, metav1.GetOptions{})
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

func (p *Provisioner) getSecretWithBackoff(namespace, name string) (secret *v1.Secret, err error) {
	logger.Infof("getting secret %q", namespace+"/"+name)

	if len(name) == 0 || len(namespace) == 0 {
		return nil, errors.New("secret name and/or namespace is missing")
	}

	secretClient := p.context.Clientset.CoreV1().Secrets(namespace)
	// Retry Get() with backoff.  Errors other than IsNotFound are ignored.
	err = wait.ExponentialBackoff(backoff, func() (done bool, err error) {
		secret, err = secretClient.Get(name, metav1.GetOptions{})
		if err == nil {
			return true, nil
		}
		if kerrors.IsNotFound(err) {
			return true, err
		}
		logger.Errorf("error getting class %q, retrying. %v", name, err)
		return false, nil
	})
	return
}
