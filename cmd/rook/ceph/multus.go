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

package ceph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
	"github.com/vishvananda/netlink"
	"gopkg.in/yaml.v2"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Main code
var multusControllerCmd = &cobra.Command{
	Use:   "multus-hostnet",
	Short: "Runs a daemonset to ensure multus connectiviy is present on hostnet",
}
var multusSetupCmd = &cobra.Command{
	Use:   "multus-setup",
	Short: "Called by controller to run a job that migrates the multus interface into the host network namespace.",
}
var multusTeardownCmd = &cobra.Command{
	Use:   "multus-teardown",
	Short: "Called by controller to run a job that removes the migrated multus interface from the host network namespace.",
}

// Main code
func init() {
	flags.SetFlagsFromEnv(multusControllerCmd.Flags(), rook.RookEnvVarPrefix)
	multusControllerCmd.RunE = multusJobController

	flags.SetFlagsFromEnv(multusSetupCmd.Flags(), rook.RookEnvVarPrefix)
	multusSetupCmd.RunE = multusSetup

	flags.SetFlagsFromEnv(multusTeardownCmd.Flags(), rook.RookEnvVarPrefix)
	multusTeardownCmd.RunE = multusTeardown
}

// controller utility
var (
	//go:embed template/setup-job.yaml
	setupJobTemplate string

	//go:embed template/teardown-job.yaml
	teardownJobTemplate string
)

// TODO organize
const (
	multusAnnotation    = "k8s.v1.cni.cncf.io/networks"
	networksAnnotation  = "k8s.v1.cni.cncf.io/networks-status"
	migrationAnnotation = "multus-migration"
)

// Main code
func multusJobController(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusControllerCmd.Flags())

	controllerName, found := os.LookupEnv("CONTROLLER_NAME")
	if !found {
		return errors.New("CONTROLLER_NAME environment variable not found")
	}
	controllerNamespace, found := os.LookupEnv("CONTROLLER_NAME")
	if !found {
		return errors.New("CONTROLLER_NAME environment variable not found")
	}

	// Set up signal handler, so that a clean up procedure will be run if the pod running this code is deleted.
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM)

	k8sClient, err := setupK8sClient()
	if err != nil {
		return errors.Wrap(err, "failed to set up k8s client")
	}

	var jobParams jobParameters
	// The pod may start running before the pod data is available to the api server.
	wait.Poll(time.Second, 20*time.Second, func() (bool, error) {
		if err := jobParams.getControllerParams(k8sClient, controllerName, controllerNamespace); err != nil {
			return false, nil
		}
		return true, nil
	})

	err = runSetupJob(k8sClient, jobParams)
	if err != nil {
		return errors.Wrap(err, "failed to run setup job")
	}

	<-signalChan
	logger.Info("Running teardown job")
	err = jobParams.getMigratedInterfaceName(k8sClient, controllerName, controllerNamespace)
	if err != nil {
		return errors.Wrap(err, "failed to get migrated interface name")
	}
	fmt.Printf("Removing multus interface %s", jobParams.MigratedInterface)
	runTeardownJob(k8sClient, jobParams)

	return nil
}

// job utility
type jobParameters struct {
	ControllerName      string
	ControllerNamespace string
	NodeName            string
	ControllerIP        string
	MultusInterface     string
	MigratedInterface   string
}

// job utility
func (params *jobParameters) getControllerParams(clientset *kubernetes.Clientset, controllerName, controllerNamespace string) error {
	pod, err := clientset.CoreV1().Pods(controllerNamespace).Get(context.TODO(), controllerName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get pod")
	}

	multusNetworkName, found := pod.ObjectMeta.Annotations[multusAnnotation]
	if !found {
		return errors.New("failed to find multus annotation")
	}
	multusConf, err := getMultusConfs(pod)
	if err != nil {
		return errors.Wrap(err, "failed to get multus configuration")
	}
	multusIfaceName, err := findMultusInterfaceName(multusConf, multusNetworkName, pod.ObjectMeta.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to get multus interface name")
	}

	params.ControllerName = pod.ObjectMeta.Name
	params.ControllerNamespace = pod.ObjectMeta.Namespace
	params.ControllerIP = pod.Status.PodIP
	params.NodeName = pod.Spec.NodeName
	params.MultusInterface = multusIfaceName

	return nil
}

// job utility
func (params *jobParameters) getMigratedInterfaceName(clientset *kubernetes.Clientset, controllerName, controllerNamespace string) error {
	pod, err := clientset.CoreV1().Pods(controllerNamespace).Get(context.TODO(), controllerName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get pod")
	}
	iface, found := pod.ObjectMeta.Annotations[migrationAnnotation]
	if !found {
		return errors.Wrap(err, "failed to get multus annotation")
	}
	params.MigratedInterface = iface
	return nil
}

// Main code
// k8s utility
func setupK8sClient() (*kubernetes.Clientset, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	// creates the clientset
	return kubernetes.NewForConfig(config)
}

// job utility that can be replace with jobParams struct
type templateParam struct {
	NodeName       string
	Namespace      string
	HolderIP       string
	MultusIface    string
	ControllerName string
	MigratedIface  string
}

// job utility
func templateToJob(name, templateData string, p templateParam) (*batch.Job, error) {
	var job batch.Job
	t, err := loadTemplate(name, templateData, p)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load job template")
	}

	err = yaml.Unmarshal([]byte(t), &job)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal job template")
	}
	return &job, nil
}

// job utility
func loadTemplate(name, templateData string, p templateParam) ([]byte, error) {
	var writer bytes.Buffer
	t := template.New(name)
	t, err := t.Parse(templateData)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse template %v", name)
	}
	err = t.Execute(&writer, p)
	return writer.Bytes(), err
}

// job utility
func runReplaceableJob(ctx context.Context, clientset kubernetes.Interface, job *batch.Job) error {
	// check if the job was already created and what its status is
	existingJob, err := clientset.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
	if err != nil && !k8sErrors.IsNotFound(err) {
		return err
	} else if err == nil {
		// delete the job that already exists from a previous run
		err := clientset.BatchV1().Jobs(existingJob.Namespace).Delete(ctx, existingJob.Name, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("failed to remove job %s. %+v", job.Name, err)
		}
		// Wait for delete to complete before continuing
		err = wait.Poll(time.Second, 20*time.Second, func() (bool, error) {
			_, err := clientset.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
			if err != nil && !k8sErrors.IsNotFound(err) {
				return false, err
			} else if err == nil {
				// Job resource hasn't been deleted yet.
				return false, nil
			}
			return true, nil
		})
	}

	_, err = clientset.BatchV1().Jobs(job.Namespace).Create(ctx, job, metav1.CreateOptions{})
	return err
}

// job utility
func WaitForJobCompletion(ctx context.Context, clientset kubernetes.Interface, job *batch.Job, timeout time.Duration) error {
	return wait.Poll(5*time.Second, timeout, func() (bool, error) {
		job, err := clientset.BatchV1().Jobs(job.Namespace).Get(ctx, job.Name, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("failed to detect job %s. %+v", job.Name, err)
		}

		// if the job is still running, allow it to continue to completion
		if job.Status.Active > 0 {
			return false, nil
		}
		if job.Status.Failed > 0 {
			return false, fmt.Errorf("job %s failed", job.Name)
		}
		if job.Status.Succeeded > 0 {
			return true, nil
		}
		return false, nil
	})
}

// controller utility
func runSetupJob(clientset *kubernetes.Clientset, params jobParameters) error {
	pJob, err := templateToJob("setup-job", setupJobTemplate, templateParam{
		NodeName:       params.NodeName,
		ControllerName: params.ControllerName,
		Namespace:      params.ControllerNamespace,
		HolderIP:       params.ControllerIP,
		MultusIface:    params.MultusInterface,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create job template")
	}

	err = runReplaceableJob(context.TODO(), clientset, pJob)
	if err != nil {
		return errors.Wrap(err, "failed to run job")
	}

	err = WaitForJobCompletion(context.TODO(), clientset, pJob, time.Minute)
	if err != nil {
		return errors.Wrap(err, "failed to complete job")
	}
	return nil
}

// controller utility
func runTeardownJob(clientset *kubernetes.Clientset, params jobParameters) error {
	pJob, err := templateToJob("teardown-job", teardownJobTemplate, templateParam{
		NodeName:       params.NodeName,
		ControllerName: params.ControllerName,
		Namespace:      params.ControllerNamespace,
		HolderIP:       params.ControllerIP,
		MultusIface:    params.MultusInterface,
		MigratedIface:  params.MigratedInterface,
	})
	if err != nil {
		return errors.Wrap(err, "failed to create job from template")
	}

	err = runReplaceableJob(context.TODO(), clientset, pJob)
	if err != nil {
		return errors.Wrap(err, "failed to run job")
	}
	return nil
}

// This is part of network utility
var InterfaceNotFound = errors.New("Interface with matching IP not found")

// This is part of multus utility
type multusNetConfiguration struct {
	NetworkName   string   `json:"name"`
	InterfaceName string   `json:"interface"`
	Ips           []string `json:"ips"`
}

// This is a multus utility
func getMultusConfs(pod *corev1.Pod) ([]multusNetConfiguration, error) {
	var multusConfs []multusNetConfiguration
	if val, ok := pod.ObjectMeta.Annotations[networksAnnotation]; ok {
		err := json.Unmarshal([]byte(val), &multusConfs)
		if err != nil {
			return multusConfs, errors.Wrap(err, "failed to unmarshal json")
		}
		return multusConfs, nil
	}
	return multusConfs, errors.Errorf("failed to find multus annotation for pod %q in namespace %q", pod.ObjectMeta.Name, pod.ObjectMeta.Namespace)
}

// This is a network utility
func findInterface(interfaces []net.Interface, ipStr string) (string, error) {
	var ifaceName string

	for _, iface := range interfaces {
		link, err := netlink.LinkByName(iface.Name)
		if err != nil {
			return ifaceName, errors.Wrap(err, "failed to get link")
		}
		if link == nil {
			return ifaceName, errors.New("failed to find link")
		}

		addrs, err := netlink.AddrList(link, 0)
		if err != nil {
			return ifaceName, errors.Wrap(err, "failed to get address from link")
		}

		for _, addr := range addrs {
			if addr.IP.String() == ipStr {
				linkAttrs := link.Attrs()
				if linkAttrs != nil {
					ifaceName = linkAttrs.Name
				}
				return ifaceName, nil
			}
		}
	}

	return ifaceName, InterfaceNotFound
}

// This is a multus utility
func findMultusInterfaceName(multusConfs []multusNetConfiguration, multusName, multusNamespace string) (string, error) {

	// The network name includes its namespace.
	multusNetwork := fmt.Sprintf("%s/%s", multusNamespace, multusName)

	for _, multusConf := range multusConfs {
		if multusConf.NetworkName == multusNetwork {
			return multusConf.InterfaceName, nil
		}
	}
	return "", errors.New("failed to find multus network configuration")
}

// setup utility
const (
	ifBase = "mlink"
	nsDir  = "/var/run/netns"
)

// main code
func multusSetup(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusSetupCmd.Flags())

	holderIP, found := os.LookupEnv("HOLDER_IP")
	if !found {
		return errors.New("HOLDER_IP environment variable not found")
	}

	multusLinkName, found := os.LookupEnv("MULTUS_IFACE")
	if !found {
		return errors.New("MULTUS_IFACE environment variable not found")
	}
	logger.Info("The multus interface to migrate is: %s\n", multusLinkName)

	controllerName, found := os.LookupEnv("CONTROLLER_NAME")
	if !found {
		return errors.New("CONTROLLER_NAME environment variable not found")
	}
	controllerNamespace, found := os.LookupEnv("CONTROLLER_NAMESPACE")
	if !found {
		return errors.New("CONTROLLER_NAMESPACE environment variable not found")
	}

	fmt.Println("Setting up k8s client")
	k8sClient, err := setupK8sClient()
	if err != nil {
		return errors.Wrap(err, "failed to set up k8s client")
	}

	holderNS, err := determineNetNS(holderIP)
	if err != nil {
		return errors.Wrap(err, "failed to determine holder network namespace")
	}

	hostNS, err := ns.GetCurrentNS()
	if err != nil {
		return errors.Wrap(err, "failed to determine host network namespace")
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return errors.Wrap(err, "failed to get interfaces in hostnet")
	}
	newLinkName, err := determineNewLinkName(interfaces)
	if err != nil {
		return errors.Wrap(err, "failed to determine new link name")
	}

	err = annotateController(k8sClient, controllerName, controllerNamespace, newLinkName)
	if err != nil {
		return errors.Wrap(err, "failed to annotate controller")
	}

	netConfig, err := getNetworkConfig(holderNS, multusLinkName)
	if err != nil {
		return errors.Wrap(err, "failed to determine multus config")
	}

	err = migrateInterface(holderNS, hostNS, multusLinkName, newLinkName)
	if err != nil {
		return errors.Wrap(err, "failed to migrate multus interface")
	}

	err = configureInterface(hostNS, newLinkName, netConfig)
	if err != nil {
		return errors.Wrap(err, "failed to configure migrated interface")
	}

	return nil
}

// network utility
func determineNetNS(ip string) (ns.NetNS, error) {
	var netNS ns.NetNS
	nsFiles, err := ioutil.ReadDir(nsDir)
	if err != nil {
		return netNS, errors.Wrap(err, "failed to read netns files")
	}

	for _, nsFile := range nsFiles {
		var foundNS bool

		netNS, err := ns.GetNS(filepath.Join(nsDir, nsFile.Name()))
		if err != nil {
			return netNS, errors.Wrap(err, "failed to get network namespace")
		}

		err = netNS.Do(func(ns ns.NetNS) error {
			interfaces, err := net.Interfaces()
			if err != nil {
				return errors.Wrap(err, "failed to list interfaces")
			}

			iface, err := findInterface(interfaces, ip)
			if err != nil {
				return errors.Wrap(err, "failed to find needed interface")
			}
			if iface != "" {
				foundNS = true
				return nil
			}
			return nil
		})

		if err != nil {
			// Don't quit, just keep looking.
			logger.Debugf("error occurred while looking for network namespace: %v; continuing search\n", err)
			continue
		}

		if foundNS {
			return netNS, nil
		}
	}

	return netNS, errors.New("failed to find network namespace")
}

// setup utility
func determineNewLinkName(interfaces []net.Interface) (string, error) {
	var newLinkName string

	linkNumber := -1
	for _, iface := range interfaces {
		if idStrs := strings.Split(iface.Name, ifBase); len(idStrs) > 1 {
			id, err := strconv.Atoi(idStrs[1])
			if err != nil {
				return newLinkName, errors.Wrap(err, "failed to convert string to integer")
			}
			if id > linkNumber {
				linkNumber = id
			}
		}
	}
	linkNumber += 1

	newLinkName = fmt.Sprintf("%s%d", ifBase, linkNumber)
	fmt.Printf("new multus link name determined: %q\n", newLinkName)

	return newLinkName, nil
}

// network utility
type netConfig struct {
	Addrs  []netlink.Addr
	Routes []netlink.Route
}

// network utility
func getNetworkConfig(netNS ns.NetNS, linkName string) (netConfig, error) {
	var conf netConfig

	err := netNS.Do(func(ns ns.NetNS) error {
		link, err := netlink.LinkByName(linkName)
		if err != nil {
			return errors.Wrap(err, "failed to get link")
		}

		conf.Addrs, err = netlink.AddrList(link, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get address from link")
		}

		conf.Routes, err = netlink.RouteList(link, 0)
		if err != nil {
			return errors.Wrap(err, "failed to get routes from link")
		}

		return nil
	})

	if err != nil {
		return conf, errors.Wrap(err, "failed to get network namespace")
	}

	return conf, nil
}

// network utlity
func migrateInterface(holderNS, hostNS ns.NetNS, multusLinkName, newLinkName string) error {
	err := holderNS.Do(func(ns.NetNS) error {

		link, err := netlink.LinkByName(multusLinkName)
		if err != nil {
			return errors.Wrap(err, "failed to get multus link")
		}

		if err := netlink.LinkSetDown(link); err != nil {
			return errors.Wrap(err, "failed to set link down")
		}

		if err := netlink.LinkSetName(link, newLinkName); err != nil {
			return errors.Wrap(err, "failed to rename link")
		}

		// After renaming the link, the link object must be updated or netlink will get confused.
		link, err = netlink.LinkByName(newLinkName)
		if err != nil {
			return errors.Wrap(err, "failed to get link")
		}

		if err = netlink.LinkSetNsFd(link, int(hostNS.Fd())); err != nil {
			return errors.Wrap(err, "failed to move interface to host namespace")
		}

		return nil
	})

	if err != nil {
		return errors.Wrap(err, "failed to migrate multus interface")
	}
	return nil
}

//  network utility
func configureInterface(hostNS ns.NetNS, linkName string, conf multusConfig) error {
	err := hostNS.Do(func(ns.NetNS) error {
		link, err := netlink.LinkByName(linkName)
		if err != nil {
			return errors.Wrap(err, "failed to get interface on host namespace")
		}
		for _, addr := range conf.Addrs {
			// The IP address label must be changed to the new interface name
			// for the AddrAdd call to succeed.
			addr.Label = linkName
			if err := netlink.AddrAdd(link, &addr); err != nil {
				return errors.Wrap(err, "failed to configure ip address on interface")
			}
		}

		//for _, route := range conf.Routes {
		//	if err := netlink.RouteAdd(&route); err != nil {
		//		return errors.Wrap(err, "failed to configure route")
		//	}
		//}

		if err := netlink.LinkSetUp(link); err != nil {
			return errors.Wrap(err, "failed to set link up")
		}
		return nil
	})

	if err != nil {
		return errors.Wrap(err, "failed to configure multus interface on host namespace")
	}
	return nil
}

// setup utility
func annotateController(k8sClient *kubernetes.Clientset, controllerName, controllerNamespace, migratedLinkName string) error {
	pod, err := k8sClient.CoreV1().Pods(controllerNamespace).Get(context.TODO(), controllerName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to get controller pod")
	}

	pod.ObjectMeta.Annotations["multus-migration"] = migratedLinkName

	_, err = k8sClient.CoreV1().Pods(controllerNamespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to update controller pod")
	}

	return nil
}

// main code
func multusTeardown(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(multusTeardownCmd.Flags())

	iface, found := os.LookupEnv("MIGRATED_IFACE")
	if !found {
		return errors.New("MIGRATED_IFACE environment variable not found")
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		return errors.Wrap(err, "failed to get multus network interface")
	}

	err = netlink.LinkDel(link)
	if err != nil {
		return errors.Wrap(err, "failed to delete multus network interface")
	}
	return nil
}
