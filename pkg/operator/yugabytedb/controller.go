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

package yugabytedb

import (
	"fmt"
	"reflect"
	"strings"

	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	yugabytedbv1alpha1 "github.com/rook/rook/pkg/apis/yugabytedb.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName          = "ybcluster"
	customResourceNamePlural    = "ybclusters"
	masterName                  = "yb-master"
	masterNamePlural            = "yb-masters"
	tserverName                 = "yb-tserver"
	tserverNamePlural           = "yb-tservers"
	masterUIServiceName         = "yb-master-ui"
	tserverUIServiceName        = "yb-tserver-ui"
	masterUIPortDefault         = int32(7000)
	masterUIPortName            = "yb-master-ui"
	masterRPCPortDefault        = int32(7100)
	masterRPCPortName           = "yb-master-rpc"
	tserverUIPortDefault        = int32(9000)
	tserverUIPortName           = "yb-tserver-ui"
	tserverRPCPortDefault       = int32(9100)
	tserverRPCPortName          = "yb-tserver-rpc"
	tserverCassandraPortDefault = int32(9042)
	tserverCassandraPortName    = "ycql"
	tserverRedisPortDefault     = int32(6379)
	tserverRedisPortName        = "yedis"
	tserverPostgresPortDefault  = int32(5433)
	tserverPostgresPortName     = "ysql"
	masterContainerUIPortName   = "master-ui"
	masterContainerRPCPortName  = "master-rpc"
	tserverContainerUIPortName  = "tserver-ui"
	tserverContainerRPCPortName = "tserver-rpc"
	uiPortName                  = "ui"
	rpcPortName                 = "rpc-port"
	cassandraPortName           = "cassandra"
	redisPortName               = "redis"
	postgresPortName            = "postgres"
	volumeMountPath             = "/mnt/data0"
	envGetHostsFrom             = "GET_HOSTS_FROM"
	envGetHostsFromVal          = "dns"
	envPodIP                    = "POD_IP"
	envPodIPVal                 = "status.podIP"
	envPodName                  = "POD_NAME"
	envPodNameVal               = "metadata.name"
	yugabyteDBImageName         = "yugabytedb/yugabyte:2.0.10.0-b4"
)

var ClusterResource = k8sutil.CustomResource{
	Name:    customResourceName,
	Plural:  customResourceNamePlural,
	Group:   yugabytedbv1alpha1.CustomResourceGroup,
	Version: yugabytedbv1alpha1.Version,
	Kind:    reflect.TypeOf(yugabytedbv1alpha1.YBCluster{}).Name(),
}

type ClusterController struct {
	context        *clusterd.Context
	containerImage string
}

func NewClusterController(context *clusterd.Context, containerImage string) *ClusterController {
	return &ClusterController{
		context:        context,
		containerImage: containerImage,
	}
}

type cluster struct {
	context     *clusterd.Context
	name        string
	namespace   string
	spec        yugabytedbv1alpha1.YBClusterSpec
	annotations rookv1alpha2.Annotations
	ownerRef    metav1.OwnerReference
}

type clusterPorts struct {
	masterPorts, tserverPorts serverPorts
}

type serverPorts struct {
	ui, rpc, cassandra, redis, postgres int32
}

func NewCluster(c *yugabytedbv1alpha1.YBCluster, context *clusterd.Context) *cluster {
	return &cluster{
		context:     context,
		name:        c.Name,
		namespace:   c.Namespace,
		spec:        c.Spec,
		annotations: c.Spec.Annotations,
		ownerRef:    clusterOwnerRef(c.Name, string(c.UID)),
	}
}

func clusterOwnerRef(name, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", ClusterResource.Group, ClusterResource.Version),
		Kind:               ClusterResource.Kind,
		Name:               name,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
	}
}

func (c *ClusterController) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.OnAdd,
		UpdateFunc: c.OnUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching yugabytedb clusters in all namespaces")
	go k8sutil.WatchCR(ClusterResource, namespace, resourceHandlerFuncs, c.context.RookClientset.YugabytedbV1alpha1().RESTClient(), &yugabytedbv1alpha1.YBCluster{}, stopCh)

	return nil
}

func (c *ClusterController) onDelete(obj interface{}) {
	cluster, ok := obj.(*yugabytedbv1alpha1.YBCluster)
	if !ok {
		return
	}
	cluster = cluster.DeepCopy()
	logger.Infof("cluster %s deleted from namespace %s", cluster.Name, cluster.Namespace)
}

func validateClusterSpec(spec yugabytedbv1alpha1.YBClusterSpec) error {

	if spec.Master.Replicas < 1 {
		return fmt.Errorf("invalid Master replica count: %d. Must be at least 1", spec.Master.Replicas)
	}

	if spec.TServer.Replicas < 1 {
		return fmt.Errorf("invalid TServer replica count: %d. Must be at least 1", spec.TServer.Replicas)
	}

	if _, err := getPortsFromSpec(spec.Master.Network); err != nil {
		return err
	}

	if _, err := getPortsFromSpec(spec.TServer.Network); err != nil {
		return err
	}

	if &spec.Master.VolumeClaimTemplate == nil {
		return fmt.Errorf("VolumeClaimTemplate unavailable in Master spec.")
	}

	if &spec.TServer.VolumeClaimTemplate == nil {
		return fmt.Errorf("VolumeClaimTemplate unavailable in TServer spec.")
	}

	return nil
}

func createAppLabels(label string) map[string]string {
	return map[string]string{
		k8sutil.AppAttr: label,
	}
}

func createServicePorts(cluster *cluster, isTServerService bool) []v1.ServicePort {
	var servicePorts []v1.ServicePort

	if !isTServerService {
		ports, _ := getPortsFromSpec(cluster.spec.Master.Network)

		servicePorts = []v1.ServicePort{
			{
				Name:       uiPortName,
				Port:       ports.masterPorts.ui,
				TargetPort: intstr.FromInt(int(ports.masterPorts.ui)),
			},
			{
				Name:       rpcPortName,
				Port:       ports.masterPorts.rpc,
				TargetPort: intstr.FromInt(int(ports.masterPorts.rpc)),
			},
		}
	} else {
		ports, _ := getPortsFromSpec(cluster.spec.TServer.Network)

		tserverUIPort := ports.tserverPorts.ui

		if tserverUIPort <= 0 {
			tserverUIPort = tserverUIPortDefault
		}

		servicePorts = []v1.ServicePort{
			{
				Name:       uiPortName,
				Port:       tserverUIPort,
				TargetPort: intstr.FromInt(int(tserverUIPort)),
			},
			{
				Name:       rpcPortName,
				Port:       ports.tserverPorts.rpc,
				TargetPort: intstr.FromInt(int(ports.tserverPorts.rpc)),
			},
			{
				Name:       cassandraPortName,
				Port:       ports.tserverPorts.cassandra,
				TargetPort: intstr.FromInt(int(ports.tserverPorts.cassandra)),
			},
			{
				Name:       redisPortName,
				Port:       ports.tserverPorts.redis,
				TargetPort: intstr.FromInt(int(ports.tserverPorts.redis)),
			},
			{
				Name:       postgresPortName,
				Port:       ports.tserverPorts.postgres,
				TargetPort: intstr.FromInt(int(ports.tserverPorts.postgres)),
			},
		}
	}

	return servicePorts
}

func createUIServicePorts(ports *clusterPorts, isTServerService bool) []v1.ServicePort {
	var servicePorts []v1.ServicePort

	if !isTServerService {
		servicePorts = []v1.ServicePort{
			{
				Name:       uiPortName,
				Port:       ports.masterPorts.ui,
				TargetPort: intstr.FromInt(int(ports.masterPorts.ui)),
			},
		}
	} else {
		if ports.tserverPorts.ui > 0 {
			servicePorts = []v1.ServicePort{
				{
					Name:       uiPortName,
					Port:       ports.tserverPorts.ui,
					TargetPort: intstr.FromInt(int(ports.tserverPorts.ui)),
				},
			}
		} else {
			servicePorts = nil
		}
	}

	return servicePorts
}

func getPortsFromSpec(networkSpec yugabytedbv1alpha1.NetworkSpec) (clusterPort *clusterPorts, err error) {
	ports := clusterPorts{}

	for _, p := range networkSpec.Ports {
		switch p.Name {
		case masterUIPortName:
			ports.masterPorts.ui = p.Port
		case masterRPCPortName:
			ports.masterPorts.rpc = p.Port
		case tserverUIPortName:
			ports.tserverPorts.ui = p.Port
		case tserverRPCPortName:
			ports.tserverPorts.rpc = p.Port
		case tserverCassandraPortName:
			ports.tserverPorts.cassandra = p.Port
		case tserverRedisPortName:
			ports.tserverPorts.redis = p.Port
		case tserverPostgresPortName:
			ports.tserverPorts.postgres = p.Port
		default:
			return &clusterPorts{}, fmt.Errorf("Invalid port name: %s. Must be one of: [%s, %s, %s, %s, %s, %s, %s]", p.Name,
				masterUIPortName, masterRPCPortName, tserverUIPortName, tserverRPCPortName, tserverCassandraPortName,
				tserverRedisPortName, tserverPostgresPortName)
		}
	}

	if ports.masterPorts.ui == 0 {
		ports.masterPorts.ui = masterUIPortDefault
	}

	if ports.masterPorts.rpc == 0 {
		ports.masterPorts.rpc = masterRPCPortDefault
	}

	if ports.tserverPorts.rpc == 0 {
		ports.tserverPorts.rpc = tserverRPCPortDefault
	}

	if ports.tserverPorts.cassandra == 0 {
		ports.tserverPorts.cassandra = tserverCassandraPortDefault
	}

	if ports.tserverPorts.redis == 0 {
		ports.tserverPorts.redis = tserverRedisPortDefault
	}

	if ports.tserverPorts.postgres == 0 {
		ports.tserverPorts.postgres = tserverPostgresPortDefault
	}

	return &ports, nil
}

func createMasterContainerCommand(namespace, serviceName string, masterCompleteName string, grpcPort, replicas int32) []string {
	command := []string{
		"/home/yugabyte/bin/yb-master",
		fmt.Sprintf("--fs_data_dirs=%s", volumeMountPath),
		fmt.Sprintf("--rpc_bind_addresses=$(POD_IP):%d", grpcPort),
		fmt.Sprintf("--server_broadcast_addresses=$(POD_NAME).%s:%d", serviceName, grpcPort),
		"--use_private_ip=never",
		fmt.Sprintf("--master_addresses=%s", getMasterAddresses(masterCompleteName, serviceName, namespace, replicas, grpcPort)),
		"--enable_ysql=true",
		fmt.Sprintf("--replication_factor=%d", replicas),
		"--logtostderr",
	}
	return command
}

func createTServerContainerCommand(namespace, serviceName, masterServiceName string, masterCompleteName string, masterGRPCPort, tserverGRPCPort, pgsqlPort, replicas int32) []string {
	command := []string{
		"/home/yugabyte/bin/yb-tserver",
		fmt.Sprintf("--fs_data_dirs=%s", volumeMountPath),
		fmt.Sprintf("--rpc_bind_addresses=$(POD_IP):%d", tserverGRPCPort),
		fmt.Sprintf("--server_broadcast_addresses=$(POD_NAME).%s:%d", serviceName, tserverGRPCPort),
		fmt.Sprintf("--pgsql_proxy_bind_address=$(POD_IP):%d", pgsqlPort),
		"--use_private_ip=never",
		fmt.Sprintf("--tserver_master_addrs=%s", getMasterAddresses(masterCompleteName, masterServiceName, namespace, replicas, masterGRPCPort)),
		"--enable_ysql=true",
		"--logtostderr",
	}
	return command
}

func getMasterAddresses(masterCompleteName string, serviceName string, namespace string, replicas int32, masterGRPCPort int32) string {
	masterAddrs := []string{}
	for i := int32(0); i < replicas; i++ {
		masterAddrs = append(masterAddrs, fmt.Sprintf("%s-%d.%s.%s.svc.cluster.local:%d", masterCompleteName, i, serviceName, namespace, masterGRPCPort))
	}
	return strings.Join(masterAddrs, ",")
}

func createMasterContainerPortsList(clusterPortsSpec *clusterPorts) []v1.ContainerPort {
	ports := []v1.ContainerPort{
		{
			Name:          masterContainerUIPortName,
			ContainerPort: int32(clusterPortsSpec.masterPorts.ui),
		},
		{
			Name:          masterContainerRPCPortName,
			ContainerPort: int32(clusterPortsSpec.masterPorts.rpc),
		},
	}

	return ports
}

func createTServerContainerPortsList(clusterPortsSpec *clusterPorts) []v1.ContainerPort {
	tserverUIPort := int32(clusterPortsSpec.tserverPorts.ui)

	if tserverUIPort <= 0 {
		tserverUIPort = tserverUIPortDefault
	}

	ports := []v1.ContainerPort{
		{
			Name:          tserverContainerUIPortName,
			ContainerPort: tserverUIPort,
		},
		{
			Name:          tserverContainerRPCPortName,
			ContainerPort: int32(clusterPortsSpec.tserverPorts.rpc),
		},
		{
			Name:          cassandraPortName,
			ContainerPort: int32(clusterPortsSpec.tserverPorts.cassandra),
		},
		{
			Name:          redisPortName,
			ContainerPort: int32(clusterPortsSpec.tserverPorts.redis),
		},
		{
			Name:          postgresPortName,
			ContainerPort: int32(clusterPortsSpec.tserverPorts.postgres),
		},
	}

	return ports
}

func (c *cluster) addCRNameSuffix(str string) string {
	return fmt.Sprintf("%s-%s", str, c.name)
}
