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
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	k8sutil "github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/ghodss/yaml"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
		return "", errors.Wrapf(err, "failed to parse template %v", name)
	}
	err = t.Execute(&writer, p)
	return writer.String(), err
}

func templateToService(name, templatePath string, p templateParam) (*corev1.Service, error) {
	var svc corev1.Service
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load service template")
	}

	err = yaml.Unmarshal([]byte(t), &svc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal service template")
	}
	return &svc, nil
}

func templateToStatefulSet(name, templatePath string, p templateParam) (*apps.StatefulSet, error) {
	var ss apps.StatefulSet
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load statefulset template")
	}

	err = yaml.Unmarshal([]byte(t), &ss)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal statefulset template")
	}
	return &ss, nil
}

func templateToDaemonSet(name, templatePath string, p templateParam) (*apps.DaemonSet, error) {
	var ds apps.DaemonSet
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load daemonset template")
	}

	err = yaml.Unmarshal([]byte(t), &ds)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal daemonset template")
	}
	return &ds, nil
}

func templateToDeployment(name, templatePath string, p templateParam) (*apps.Deployment, error) {
	var ds apps.Deployment
	t, err := loadTemplate(name, templatePath, p)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load deployment template")
	}

	err = yaml.Unmarshal([]byte(t), &ds)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal deployment template")
	}
	return &ds, nil
}

func getToleration(provisioner bool) []corev1.Toleration {
	// Add toleration if any
	tolerations := []corev1.Toleration{}
	var err error
	tolerationsRaw := ""
	if provisioner {
		tolerationsRaw = os.Getenv(provisionerTolerationsEnv)
	} else {
		tolerationsRaw = os.Getenv(pluginTolerationsEnv)
	}
	if tolerationsRaw != "" {
		tolerations, err = k8sutil.YamlToTolerations(tolerationsRaw)
		if err != nil {
			logger.Warningf("failed to parse %q. %v", tolerationsRaw, err)
		}
	}
	for i := range tolerations {
		if tolerations[i].Key == "" {
			tolerations[i].Operator = corev1.TolerationOpExists
		}

		if tolerations[i].Operator == corev1.TolerationOpExists {
			tolerations[i].Value = ""
		}
	}
	return tolerations
}

func getNodeAffinity(provisioner bool) *corev1.NodeAffinity {
	// Add NodeAffinity if any
	nodeAffinity := ""
	if provisioner {
		nodeAffinity = os.Getenv(provisionerNodeAffinityEnv)
	} else {
		nodeAffinity = os.Getenv(pluginNodeAffinityEnv)
	}
	if nodeAffinity != "" {
		v1NodeAffinity, err := k8sutil.GenerateNodeAffinity(nodeAffinity)
		if err != nil {
			logger.Warningf("failed to parse %q. %v", nodeAffinity, err)
		}
		return v1NodeAffinity
	}
	return nil
}

func applyToPodSpec(pod *corev1.PodSpec, n *corev1.NodeAffinity, t []corev1.Toleration) {
	pod.Tolerations = t
	pod.Affinity = &corev1.Affinity{
		NodeAffinity: n,
	}
}

func getPortFromENV(env string, defaultPort uint16) uint16 {
	port := os.Getenv(env)
	if strings.TrimSpace(port) == "" {
		return defaultPort
	}
	p, err := strconv.ParseUint(port, 10, 64)
	if err != nil {
		logger.Debugf("failed to parse port value for env %q. using default port %d. %v", env, defaultPort, err)
		return defaultPort
	}
	if p > 65535 {
		logger.Debugf("%s port value is greater than 65535. using default port %d for %s", port, defaultPort, env)
		return defaultPort
	}
	return uint16(p)
}
