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

package multus

import (
	"context"
	_ "embed"
	"time"

	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"
)

var (
	//go:embed template/setup-job.yaml
	setupJobTemplate string

	//go:embed template/teardown-job.yaml
	teardownJobTemplate string
)

func RunSetupJob(clientset *kubernetes.Clientset, params JobParameters) error {
	pJob, err := templateToJob("setup-job", setupJobTemplate, params)
	if err != nil {
		return errors.Wrap(err, "failed to create job template")
	}

	err = runReplaceableJob(context.Background(), clientset, pJob)
	if err != nil {
		return errors.Wrap(err, "failed to run job")
	}

	err = WaitForJobCompletion(context.Background(), clientset, pJob, time.Minute)
	if err != nil {
		return errors.Wrap(err, "failed to complete job")
	}
	return nil
}

func RunTeardownJob(clientset *kubernetes.Clientset, params JobParameters) error {
	pJob, err := templateToJob("teardown-job", teardownJobTemplate, params)
	if err != nil {
		return errors.Wrap(err, "failed to create job from template")
	}

	err = runReplaceableJob(context.Background(), clientset, pJob)
	if err != nil {
		return errors.Wrap(err, "failed to run job")
	}
	return nil
}
