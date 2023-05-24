/*
Copyright 2023 The Rook Authors. All rights reserved.

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
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var (
	//go:embed nginx-pod.yaml
	nginxPodTemplate string

	//go:embed nginx-config.yaml
	nginxConfigTemplate string

	//go:embed image-pull-daemonset.yaml
	imagePullDaemonSet string

	//go:embed client-daemonset.yaml
	clientDaemonSet string
)

type webServerTemplateConfig struct {
	NetworksAnnotationValue string
	NginxImage              string
}

type imagePullTemplateConfig struct {
	NginxImage string
}

type clientTemplateConfig struct {
	ClientID                 int
	NetworksAnnotationValue  string
	NetworkNamesAndAddresses map[string]string
	NginxImage               string
}

func webServerPodName() string {
	return "multus-validation-test-web-server"
}

func imagePullAppLabel() string {
	return "app=multus-validation-test-image-pull"
}

func clientAppLabel() string {
	return "app=multus-validation-test-client"
}

type daemonsetAppType string

const imagePullDaemonSetAppType = "image pull"
const clientDaemonSetAppType = "client"

func (vt *ValidationTest) generateWebServerTemplateConfig() webServerTemplateConfig {
	return webServerTemplateConfig{
		NetworksAnnotationValue: vt.generateNetworksAnnotationValue(),
		NginxImage:              vt.NginxImage,
	}
}

func (vt *ValidationTest) generateClientTemplateConfig(clientID int, serverPublicAddr, serverClusterAddr string) clientTemplateConfig {
	netNamesAndAddresses := map[string]string{}
	if serverPublicAddr != "" {
		netNamesAndAddresses["public"] = serverPublicAddr
	}
	if serverClusterAddr != "" {
		netNamesAndAddresses["cluster"] = serverClusterAddr
	}
	return clientTemplateConfig{
		ClientID:                 clientID,
		NetworksAnnotationValue:  vt.generateNetworksAnnotationValue(),
		NetworkNamesAndAddresses: netNamesAndAddresses,
		NginxImage:               vt.NginxImage,
	}
}

func (vt *ValidationTest) generateImagePullTemplateConfig() imagePullTemplateConfig {
	return imagePullTemplateConfig{
		NginxImage: vt.NginxImage,
	}
}

func (vt *ValidationTest) generateWebServerPod() (*core.Pod, error) {
	t, err := loadTemplate("webServerPod", nginxPodTemplate, vt.generateWebServerTemplateConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to load web server pod template: %w", err)
	}

	var p core.Pod
	err = yaml.Unmarshal(t, &p)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal web server pod template: %w", err)
	}

	return &p, nil
}

func (vt *ValidationTest) generateWebServerConfigMap() (*core.ConfigMap, error) {
	t, err := loadTemplate("webServerConfigMap", nginxConfigTemplate, vt.generateWebServerTemplateConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to load web server configmap template: %w", err)
	}

	var cm core.ConfigMap
	err = yaml.Unmarshal(t, &cm)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal web server configmap template: %w", err)
	}

	return &cm, nil
}

func (vt *ValidationTest) generateImagePullDaemonSet() (*apps.DaemonSet, error) {
	t, err := loadTemplate("imagePullDaemonSet", imagePullDaemonSet, vt.generateImagePullTemplateConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to load image pull daemonset template: %w", err)
	}

	var d apps.DaemonSet
	err = yaml.Unmarshal(t, &d)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal image pull daemonset template: %w", err)
	}

	return &d, nil
}

func (vt *ValidationTest) generateClientDaemonSet(
	clientID int,
	serverPublicAddr, serverClusterAddr string,
) (*apps.DaemonSet, error) {
	t, err := loadTemplate("clientDaemonSet", clientDaemonSet, vt.generateClientTemplateConfig(clientID, serverPublicAddr, serverClusterAddr))
	if err != nil {
		return nil, fmt.Errorf("failed to load client daemonset template: %w", err)
	}

	var d apps.DaemonSet
	err = yaml.Unmarshal(t, &d)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal client daemonset template: %w", err)
	}

	return &d, nil
}

func (vt *ValidationTest) generateNetworksAnnotationValue() string {
	nets := []string{}
	if vt.PublicNetwork != "" {
		nets = append(nets, vt.PublicNetwork)
	}
	if vt.ClusterNetwork != "" {
		nets = append(nets, vt.ClusterNetwork)
	}
	return strings.Join(nets, ",")
}

func loadTemplate(name, templateFileText string, config interface{}) ([]byte, error) {
	var writer bytes.Buffer
	t := template.New(name)
	t, err := t.Parse(templateFileText)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %q: %w", name, err)
	}
	err = t.Execute(&writer, config)
	return writer.Bytes(), err
}
