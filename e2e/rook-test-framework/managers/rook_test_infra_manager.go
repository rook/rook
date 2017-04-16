package managers

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
	"github.com/dangula/rook/e2e/rook-test-framework/enums"
	"github.com/dangula/rook/e2e/rook-test-framework/objects"
	"github.com/dangula/rook/e2e/rook-test-framework/transport"
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type rookTestInfraManager struct {
	transportClient contracts.ITransportClient
	platformType    enums.RookPlatformType
	dockerized      bool
	dockerContext   *objects.DockerContext
	k8sVersion      enums.K8sVersion
	goPath		string
}

var (
	r    *rookTestInfraManager
	once sync.Once
)

const (
	dockerInfraTag string = "quay.io/quantum/rook-test"
	kubeControllerManagerJsonFileName = "kube-controller-manager.json"
	rookOperatorFileName = "rook-operator.yaml"
	manifestPath = "/etc/kubernetes/manifests"
	podSpecPath = "src/github.com/dangula/rook/e2e/pod-specs"
	scriptsPath = "src/github.com/dangula/rook/e2e/scripts"
	k8sFalsePostiveSuccessErrorMsg = "exit status 1"
	tempDockerSockMountPath = "/tmp/docker.sock"
	dockerSockPath = "/var/run/docker.sock"
)

func GetRookTestInfraManager(platformType enums.RookPlatformType, isDockerized bool, version enums.K8sVersion) (error, *rookTestInfraManager) {
	var transportClient contracts.ITransportClient
	var dockerContext objects.DockerContext
	var dockerized bool = isDockerized

	if r != nil {
		return nil, r
	}

	switch {
	case platformType == enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
	case platformType == enums.StandAlone:
		return errors.New("Unsupported Rook Platform Type"), r
	default:
		return errors.New("Unsupported Rook Platform Type"), r
	}

	once.Do(func() {
		//this is needed when test development vs boot2docker
		//dockerEnv := []string {
		//	"DOCKER_TLS_VERIFY=1",
		//	"DOCKER_HOST=tcp://192.168.99.100:2376",
		//	//"DOCKER_CERT_PATH=/Users/tyjohnson/.docker/machine/machines/default",
		//	"DOCKER_MACHINE_NAME=default"}
		dockerEnv := []string{}

		if isDockerized {
			dockerContext = objects.SetDockerContext(transport.CreateDockerClient(dockerEnv))
		}

		goPath := os.Getenv("GOPATH")

		if strings.EqualFold(goPath, "") {
			panic(errors.New("$GOPATH environment variable must be set"))
		}

		r = &rookTestInfraManager{
			platformType:    platformType,
			transportClient: transportClient,
			dockerized:      dockerized,
			dockerContext:   &dockerContext,
			k8sVersion:      version,
			goPath: goPath,
		}
	})

	return nil, r
}

func (r *rookTestInfraManager) GetRookPlatform() enums.RookPlatformType {
	return r.platformType
}

func (r *rookTestInfraManager) createRookInfraContainer(dockerInfraTag string) string {
	cmd := []string {
		"--rm", "-itd", "--net=host", "-e=\"container=docker\"", "--privileged", "--security-opt=seccomp:unconfined",
		"--cap-add=SYS_ADMIN", "-v", "/dev:/dev", "-v", "/sys:/sys", "-v", "/sys/fs/cgroup:/sys/fs/cgroup", "-v",
		"/sbin/modprobe:/sbin/modprobe", "-v", "/lib/modules:/lib/modules:rw", "-v", dockerSockPath + ":" + tempDockerSockMountPath,
		"-p", "8080", "-P", dockerInfraTag, "/sbin/init",
	}

	_, containerId, err := r.dockerContext.Get_DockerClient().Run(cmd)

	if err != nil {
		panic(err)
	}

	return containerId
}

func (r *rookTestInfraManager) verifyRookInfraContainerNotAlreadyRunning() (isRunning bool) {
	containerId := r.dockerContext.Get_ContainerId()

	if containerId != "" && r.isContainerRunning(containerId) {
		return true
	}

	return false
}

func panicIfError(err error) {
	if err != nil {
		panic(err)
	}
}

func (r *rookTestInfraManager) executeDockerCommand(containerId string, subCommand enums.DockerSubCommand, args ...string) (stdOut, stdErr string) {
	var cmdPrefix string = ""
 	var cmdArgs []string

	if subCommand != enums.Empty {
		cmdPrefix = subCommand.String()
	}

	if strings.EqualFold(containerId, "") {
		containerId = r.dockerContext.Get_ContainerId()
	}

	if subCommand == enums.Copy || subCommand == enums.Ps {
		cmdArgs = append([]string{cmdPrefix}, args...)
	} else {
		cmdArgs = append([]string{cmdPrefix, containerId}, args...)
	}

	fmt.Println("Executing the command 'docker " + strings.Join(cmdArgs, " ") + "'")

	stdOut, stdErr, err := r.dockerContext.Get_DockerClient().ExecuteCmd(cmdArgs)

	panicIfError(err)

	fmt.Println("Command succeeded")

	return stdOut, stdErr
}

func getDindScriptName () (dindScriptName string) {
	switch {
	case strings.EqualFold(r.k8sVersion.String(), enums.V1dot5.String()):
		dindScriptName = "rook-dind-cluster-v1.5.sh"
	case strings.EqualFold(r.k8sVersion.String(), enums.V1dot6.String()):
		dindScriptName = "rook-dind-cluster-v1.6.sh"
	default:
		panic(errors.New("Unsupported Kubernetes version"))
	}

	return dindScriptName
}

func (r *rookTestInfraManager) ValidateAndSetupTestPlatform() {
	if r.verifyRookInfraContainerNotAlreadyRunning() {
		return
	}

	containerId := r.dockerContext.Set_ContainerId(r.createRookInfraContainer(dockerInfraTag))

	//give docker a chance to initialize in the container
	r.executeDockerCommand("", enums.Exec, "sleep", "5")

	r.executeDockerCommand("", enums.Exec,"docker", "info")

	r.executeDockerCommand("", enums.Exec, "rm", "-rfv", dockerSockPath)

	r.executeDockerCommand("", enums.Exec, "ln", "-s", tempDockerSockMountPath, dockerSockPath)

	dindScriptName := getDindScriptName()

	r.executeDockerCommand("", enums.Copy, r.goPath + "/" + scriptsPath + "/" + dindScriptName, containerId + ":" + dindScriptName)

	r.executeDockerCommand("", enums.Exec, "chmod", "+x", dindScriptName)

	r.executeDockerCommand("", enums.Exec, "./" + dindScriptName, "up")

	//Untaint master node
	k8sClient := transport.CreateNewk8sTransportClient()

	_, _, err := k8sClient.ExecuteCmd([]string{"taint", "nodes", "--all", "dedicated-"})

	if err != nil && !strings.EqualFold(err.Error(), k8sFalsePostiveSuccessErrorMsg) {
		panic(err)
	}

	//Drain node 2
	_, _, err = k8sClient.ExecuteCmd([]string{"drain", "kube-node-2", "--force", "--ignore-daemonsets"})

	if err != nil && !strings.EqualFold(err.Error(), k8sFalsePostiveSuccessErrorMsg) {
		panic(err)
	}

	//Delete 2nd unnecessary node
	_, _, err = k8sClient.ExecuteCmd([]string{"delete", "node", "kube-node-2", "--force"})

	if err != nil && !strings.EqualFold(err.Error(), k8sFalsePostiveSuccessErrorMsg) {
		panic(err)
	}

	_, k8sMasterContainerId := r.executeDockerCommand("", enums.Ps, "--filter", "name=kube-master", "--format", "{{.ID}}")

	//Patch controller with ceph-common installed one
	r.executeDockerCommand("",enums.Copy, r.goPath + "/" + podSpecPath + "/" + kubeControllerManagerJsonFileName, k8sMasterContainerId+ ":" + manifestPath + "/" + kubeControllerManagerJsonFileName)

	//Install ceph-common on each k8s node
	r.executeDockerCommand(k8sMasterContainerId, enums.Exec,"bin/bash", "-c", "apt-get -y update && apt-get install -qqy ceph-common")

	_, k8sNode1ContainerId := r.executeDockerCommand("", enums.Ps, "--filter", "name=kube-node-1", "--format", "{{.ID}}")

	r.executeDockerCommand(k8sNode1ContainerId, enums.Exec,"bin/bash", "-c", "apt-get -qy update && apt-get install -qqy ceph-common")
}

func (r *rookTestInfraManager) InstallRook(tag string) (err error, client contracts.Irook_client) {
	//if k8
	//STEP 1 --> Create rook operator
	k8sHelp := utils.CreatK8sHelper()

	raw, _ := ioutil.ReadFile(r.goPath + "/" + podSpecPath + "/" + rookOperatorFileName)

	rawUpdated := bytes.Replace(raw, []byte("#IMAGE_PATH#"), []byte(tag), 1)
	rookOperator := string(rawUpdated)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookOperator)

	if exitCode != 0 {
		return errors.New(string(exitCode)), nil
	}

	if !k8sHelp.IsThirdPartyResourcePresent("rookcluster.rook.io") {
		fmt.Println("Rook Operator couldn't start")
	} else {
		fmt.Println("Rook Operator started")
	}

	time.Sleep(10 * time.Second)	///TODO: add real check here

	//STEP 2 --> Create rook cluster
	raw, _ = ioutil.ReadFile(r.goPath + "/src/github.com/dangula/rook/e2e/pod-specs/rook-cluster.yaml")

	rookCluster := string(raw)

	_, _, exitCode = r.transportClient.CreateWithStdin(rookCluster)

	if exitCode != 0 {
		return errors.New(string(exitCode)), nil
	}

	if !k8sHelp.IsServiceUpInNameSpace("rook-api") {
		fmt.Println("Rook Cluster couldn't start")
	} else {
		fmt.Println("Rook Cluster started")
	}

	time.Sleep(10 * time.Second)

	//STEP 3 --> Create rook client
	raw, _ = ioutil.ReadFile(r.goPath + "/src/github.com/dangula/rook/e2e/pod-specs/rook-client.yaml")

	rookClient := string(raw)

	_, _, exitCode = r.transportClient.CreateWithStdin(rookClient)

	if exitCode != 0 {
		return errors.New(string(exitCode)), nil
	}

	if !k8sHelp.IsPodRunningInNamespace("rook-client") {
		fmt.Println("Rook Client couldn't start")
	} else {
		fmt.Println("Rook Client started")
	}

	return nil, nil
}

func (r *rookTestInfraManager) isContainerRunning(containerId string) bool {
	dockerClient := r.dockerContext.Get_DockerClient()

	_, stdErr, _ := dockerClient.ExecuteCmd([]string{"ps", "--filter", "status=running", "--filter", "id=" + containerId, "--format", "\"{{.ID}}\""})

	return strings.EqualFold(stdErr, containerId)
}

func (r rookTestInfraManager) TearDownRook(client contracts.Irook_client) error {

	return nil
}

func (r rookTestInfraManager) TearDownInfrastructureCreatedEnvironment() error {
	return nil
}

func (r rookTestInfraManager) isRookInstalled() bool {
	return false
}

func (r rookTestInfraManager) CanConnectToDocker() bool {
	return false
}

func (r rookTestInfraManager) CanConnectToK8s() bool {
	return false
}

