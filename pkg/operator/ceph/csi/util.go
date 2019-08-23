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

package csi

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/ghodss/yaml"

	apps "k8s.io/api/apps/v1"
)

func loadTemplate(name, templatePath string, p templateParam) (string, error) {
	b, err := ioutil.ReadFile(templatePath)
	if err != nil {
		return "", err
	}
	data := string(b)
	var writer bytes.Buffer
	t := template.New(name)
	t, err = t.Parse(data)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %v. %+v", name, err)
	}
	err = t.Execute(&writer, p)
	return writer.String(), err
}

func templateToStatefulSet(name, templatePath string, p templateParam) (*apps.StatefulSet, error) {
	var ss apps.StatefulSet
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal([]byte(t), &ss)
	if err != nil {
		return nil, err
	}
	return &ss, nil
}

func templateToDaemonSet(name, templatePath string, p templateParam) (*apps.DaemonSet, error) {
	var ds apps.DaemonSet
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal([]byte(t), &ds)
	if err != nil {
		return nil, err
	}
	return &ds, nil
}

func templateToDeployment(name, templatePath string, p templateParam) (*apps.Deployment, error) {
	var ds apps.Deployment
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal([]byte(t), &ds)
	if err != nil {
		return nil, err
	}
	return &ds, nil
}
