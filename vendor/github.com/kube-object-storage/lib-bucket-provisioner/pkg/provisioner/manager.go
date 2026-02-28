/*
Copyright 2019 Red Hat Inc.

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

package provisioner

import (
	"context"
	"flag"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	klog "k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"

	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	informers "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/informers/externalversions"
	"github.com/kube-object-storage/lib-bucket-provisioner/pkg/provisioner/api"
)

// Provisioner wraps a custom controller which watches OBCs and manages OB, CMs, and Secrets.
type Provisioner struct {
	Name            string
	Provisioner     api.Provisioner
	claimController controller
	informerFactory informers.SharedInformerFactory
}

func initLoggers() {
	log = klogr.New().WithName(api.Domain + "/provisioner-manager")
	logD = log.V(1)
}

func initFlags() {
	klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(klogFlags)

	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		kflag := klogFlags.Lookup(f.Name)
		if kflag != nil {
			val := f.Value.String()
			kflag.Value.Set(val)
		}
	})
	if !flag.Parsed() {
		flag.Parse()
	}
}

// NewProvisioner should be called by importers of this library to
// instantiate a new provisioning obcController. This obcController will
// respond to Add / Update / Delete events by calling the passed-in
// provisioner's Provisioner and Delete methods.
// The Provisioner will be restrict to operating only to the namespace given
func NewProvisioner(
	cfg *rest.Config,
	provisionerName string,
	provisioner api.Provisioner,
	namespace string,
) (*Provisioner, error) {

	initFlags()
	initLoggers()

	libClientset := versioned.NewForConfigOrDie(cfg)
	clientset := kubernetes.NewForConfigOrDie(cfg)

	informerFactory := setupInformerFactory(libClientset, 0, namespace)

	p := &Provisioner{
		Name:            provisionerName,
		informerFactory: informerFactory,

		claimController: NewController(
			provisionerName,
			provisioner,
			clientset,
			libClientset,
			informerFactory.Objectbucket().V1alpha1().ObjectBucketClaims(),
			informerFactory.Objectbucket().V1alpha1().ObjectBuckets()),
	}

	return p, nil
}

// SetLabels allows provisioner author to provide their own resource labels.  They will be set on all
// managed resources by the provisioner (OBC, OB, CM, Secret)
func (p *Provisioner) SetLabels(labels map[string]string) []string {
	var errs []string
	for _, v := range labels {
		vErrs := validation.IsValidLabelValue(v)
		if len(errs) > 0 {
			errs = append(errs, vErrs...)
		}
	}
	if len(errs) > 0 {
		return errs
	}
	p.claimController.SetLabels(labels)
	return nil
}

// Run starts the claim and bucket controllers.
func (p *Provisioner) Run(stopCh <-chan struct{}) (err error) {
	defer klog.Flush()
	log.Info("starting provisioner", "name", p.Name)

	p.informerFactory.Start(stopCh)

	go func() {
		err = p.claimController.Start(stopCh)
	}()
	<-stopCh
	return
}

func (p *Provisioner) RunWithContext(context context.Context) (err error) {
	stopCh := make(chan struct{})

	defer klog.Flush()
	log.Info("starting provisioner", "name", p.Name)

	p.informerFactory.Start(stopCh)

	go func() {
		err = p.claimController.Start(stopCh)
	}()

	select {
	case <-stopCh:
		return err
	case <-context.Done():
		close(stopCh)
		log.Info("stopping provisioner", "name", p.Name, "reason", context.Err())
		return nil
	}
}

// setupInformerFactory generates an informer factory scoped to the given namespace if provided or
// to the cluster if empty.
func setupInformerFactory(c versioned.Interface, resyncPeriod time.Duration, ns string) (inf informers.SharedInformerFactory) {
	if len(ns) > 0 {
		return informers.NewSharedInformerFactoryWithOptions(
			c,
			resyncPeriod,
			informers.WithNamespace(ns),
		)
	}
	return informers.NewSharedInformerFactory(c, resyncPeriod)
}
