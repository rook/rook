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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Placement               PlacementConfig
}

type imagePullTemplateConfig struct {
	NodeType   string
	NginxImage string
	Placement  PlacementConfig
}

type clientTemplateConfig struct {
	NodeType                 string
	ClientType               string
	ClientID                 int
	NetworksAnnotationValue  string
	NetworkNamesAndAddresses map[string]string
	NginxImage               string
	Placement                PlacementConfig
}

func webServerPodName() string {
	return "multus-validation-test-web-server"
}

func imagePullAppLabel() string {
	return "app=multus-validation-test-image-pull"
}

func getNodeType(m *metav1.ObjectMeta) string {
	return m.GetLabels()["nodeType"]
}

func clientAppLabel() string {
	return "app=multus-validation-test-client"
}

const (
	ClientTypeOSD    = "osd"
	ClientTypeNonOSD = "other"
)

type daemonsetAppType string

const imagePullDaemonSetAppType = "image pull"
const clientDaemonSetAppType = "client"

func (vt *ValidationTest) generateWebServerTemplateConfig(placement PlacementConfig) webServerTemplateConfig {
	return webServerTemplateConfig{
		NetworksAnnotationValue: vt.generateNetworksAnnotationValue(true, true), // always on both nets
		NginxImage:              vt.NginxImage,
		Placement:               placement,
	}
}

func (vt *ValidationTest) generateClientTemplateConfig(
	attachPublic, attachCluster bool,
	serverPublicAddr, serverClusterAddr string,
	nodeType, clientType string,
	clientID int,
	placement PlacementConfig,
) clientTemplateConfig {
	netNamesAndAddresses := map[string]string{}
	if attachPublic && serverPublicAddr != "" {
		netNamesAndAddresses["public"] = serverPublicAddr
	}
	if attachCluster && serverClusterAddr != "" {
		netNamesAndAddresses["cluster"] = serverClusterAddr
	}
	return clientTemplateConfig{
		NodeType:                 nodeType,
		ClientType:               clientType,
		ClientID:                 clientID,
		NetworksAnnotationValue:  vt.generateNetworksAnnotationValue(attachPublic, attachCluster),
		NetworkNamesAndAddresses: netNamesAndAddresses,
		NginxImage:               vt.NginxImage,
		Placement:                placement,
	}
}

func (vt *ValidationTest) generateImagePullTemplateConfig(nodeType string, placement PlacementConfig) imagePullTemplateConfig {
	return imagePullTemplateConfig{
		NodeType:   nodeType,
		NginxImage: vt.NginxImage,
		Placement:  placement,
	}
}

func (vt *ValidationTest) generateWebServerPod(placement PlacementConfig) (*core.Pod, error) {
	t, err := loadTemplate("webServerPod", nginxPodTemplate, vt.generateWebServerTemplateConfig(placement))
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
	t, err := loadTemplate("webServerConfigMap", nginxConfigTemplate, vt.generateWebServerTemplateConfig(
		PlacementConfig{}, // not used for configmap
	))
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

func (vt *ValidationTest) generateImagePullDaemonSet(nodeType string, placement PlacementConfig) (*apps.DaemonSet, error) {
	t, err := loadTemplate("imagePullDaemonSet", imagePullDaemonSet, vt.generateImagePullTemplateConfig(nodeType, placement))
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
	attachPublic, attachCluster bool,
	serverPublicAddr, serverClusterAddr string,
	nodeType, clientType string,
	clientID int,
	placement PlacementConfig,
) (*apps.DaemonSet, error) {
	t, err := loadTemplate("clientDaemonSet", clientDaemonSet, vt.generateClientTemplateConfig(attachPublic, attachCluster, serverPublicAddr, serverClusterAddr, nodeType, clientType, clientID, placement))
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

func (vt *ValidationTest) generateNetworksAnnotationValue(public, cluster bool) string {
	nets := []string{}
	if public && vt.PublicNetwork != "" {
		nets = append(nets, vt.PublicNetwork)
	}
	if cluster && vt.ClusterNetwork != "" {
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
