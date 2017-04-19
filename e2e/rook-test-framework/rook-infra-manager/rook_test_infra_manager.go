package rook_infra_manager

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
	"strings"
	"sync"
	"time"
	"runtime"
	"path"
)

type rookTestInfraManager struct {
	transportClient contracts.ITransportClient
	platformType    enums.RookPlatformType
	dockerized      bool
	dockerContext   *objects.DockerContext
	k8sVersion      enums.K8sVersion
}

var (
	r    *rookTestInfraManager
	once sync.Once
	curdir string
)

const (
	kubeControllerManagerJsonFileName = "kube-controller-manager.json"
	rookOperatorFileName = "rook-operator.yaml"
	rookClusterFileName = "rook-cluster.yaml"
	rookClientFileName = "rook-client.yaml"
	manifestPath = "/etc/kubernetes/manifests"
	podSpecPath = "pod-specs"
	scriptsPath = "scripts"
	k8sFalsePostiveSuccessErrorMsg = "exit status 1"
	rookDindK8sClusterScriptv1_5 = "rook-dind-cluster-v1.5.sh"
	rookDindK8sClusterScriptv1_6 = "rook-dind-cluster-v1.6.sh"
	rookOperatorImagePodSpecTag = "#IMAGE_PATH#"
)

func GetRookTestInfraManager(platformType enums.RookPlatformType, isDockerized bool, version enums.K8sVersion) (error, *rookTestInfraManager) {
	var transportClient contracts.ITransportClient
	var dockerContext objects.DockerContext
	var dockerized bool = isDockerized

	//dont recreate singleton manager if it already exists
	if r != nil {
		return nil, r
	}

	curdir = getCurrentDirectory()

	switch platformType {
	case enums.Kubernetes:
		transportClient = transport.CreateNewk8sTransportClient()
	case enums.StandAlone:
		return errors.New("Unsupported Rook Platform Type"), r
	default:
		return errors.New("Unsupported Rook Platform Type"), r
	}

	//init singleton manager in a thread safe manner
	once.Do(func() {
		dockerEnv := []string{}

		if isDockerized {
			dockerContext = objects.SetDockerContext(transport.CreateDockerClient(dockerEnv))
		}

		r = &rookTestInfraManager{
			platformType:    platformType,
			transportClient: transportClient,
			dockerized:      dockerized,
			dockerContext:   &dockerContext,
			k8sVersion:      version,
		}
	})

	return nil, r
}

func getCurrentDirectory() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("No caller information")
	}

	return path.Dir(filename)
}

func (r *rookTestInfraManager) GetRookPlatform() enums.RookPlatformType {
	return r.platformType
}

func (r *rookTestInfraManager) executeDockerCommand(containerId string, subCommand enums.DockerSubCommand , args ...string) (stdOut, stdErr string) {
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

	if err != nil {
		panic(err)
	}

	fmt.Println("Command succeeded...")

	return stdOut, stdErr
}

func getDindScriptName (k8sVersion enums.K8sVersion) (dindScriptName string) {
	switch k8sVersion {
	case enums.V1dot5:
		dindScriptName = rookDindK8sClusterScriptv1_5
	case enums.V1dot6:
		dindScriptName = rookDindK8sClusterScriptv1_6
	default:
		panic(errors.New("Unsupported Kubernetes version: " + k8sVersion.String()))
	}

	return dindScriptName
}

func (r *rookTestInfraManager) ValidateAndSetupTestPlatform() {
	dindScriptName := getDindScriptName(r.k8sVersion)

	//make the k8s creation script executable
	cmdOut := utils.ExecuteCommand(objects.Command_Args{Command:"chmod", CmdArgs:[]string{"+x", curdir + "/" + scriptsPath + "/" + dindScriptName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(errors.New("Failed to chmod script"))
	}

	//launch the k8s creation script
	cmdOut = utils.ExecuteCommand(objects.Command_Args{Command:  curdir + "/" + scriptsPath + "/" + dindScriptName, CmdArgs:[]string{ "up"}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(errors.New("Failed to execute script"))
	}

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

	//Patch k8s controller with ceph-common installed one
	r.executeDockerCommand("",enums.Copy, curdir + "/" + podSpecPath + "/" + kubeControllerManagerJsonFileName, k8sMasterContainerId + ":" + manifestPath + "/" + kubeControllerManagerJsonFileName)

	//Install ceph-commons on each k8s node
	r.executeDockerCommand(k8sMasterContainerId, enums.Exec,"bin/bash", "-c", "apt-get -y update && apt-get install -qqy ceph-common")

	_, k8sNode1ContainerId := r.executeDockerCommand("", enums.Ps, "--filter", "name=kube-node-1", "--format", "{{.ID}}")

	r.executeDockerCommand(k8sNode1ContainerId, enums.Exec,"bin/bash", "-c", "apt-get -qy update && apt-get install -qqy ceph-common")
}

func createK8sRookOperator(k8sHelper *utils.K8sHelper, tag string) error {
	raw, err := ioutil.ReadFile(curdir + "/" + podSpecPath + "/" + rookOperatorFileName)

	if err != nil {
		return err
	}

	rawUpdated := bytes.Replace(raw, []byte(rookOperatorImagePodSpecTag), []byte(tag), 1)
	rookOperator := string(rawUpdated)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookOperator)

	if exitCode != 0 {
		return errors.New(string("Failed to create rook-operator pod; kubectl exit code = " + string(exitCode)))
	}

	if !k8sHelper.IsThirdPartyResourcePresent("cluster.rook.io") {
		fmt.Println("Rook Operator couldn't start")
	} else {
		fmt.Println("Rook Operator started")
	}

	return nil
}

func createk8sRookCluster(k8sHelper *utils.K8sHelper) error {
	raw, err := ioutil.ReadFile(curdir + "/" + podSpecPath + "/" + rookClusterFileName)

	if err != nil {
		return err
	}

	rookCluster := string(raw)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookCluster)

	if exitCode != 0 {
		return errors.New("Failed to create rook-cluster pod; kubectl exit code = " + string(exitCode))
	}

	if !k8sHelper.IsServiceUpInNameSpace("rook-api") {
		fmt.Println("Rook Cluster couldn't start")
	} else {
		fmt.Println("Rook Cluster started")
	}

	return nil
}

func (r *rookTestInfraManager) InstallRook(tag string) (err error, client contracts.Irook_client) {
	//Create rook operator
	k8sHelp := utils.CreatK8sHelper()

	createK8sRookOperator(k8sHelp, tag)

	time.Sleep(5 * time.Second)	///TODO: add real check here

	//Create rook cluster
	createk8sRookCluster(k8sHelp)

	time.Sleep(5 * time.Second)

	//STEP 3 --> Create rook client
	raw, _ := ioutil.ReadFile(curdir + "/" + podSpecPath + "/" + rookClientFileName)

	rookClient := string(raw)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookClient)

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
	dindScriptName := getDindScriptName(r.k8sVersion)

	cmdOut := utils.ExecuteCommand(objects.Command_Args{Command:"chmod", CmdArgs:[]string{"+x",  curdir + "/" + scriptsPath + "/" + dindScriptName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(errors.New("Failed to chmod script"))
	}

	cmdOut = utils.ExecuteCommand(objects.Command_Args{Command:  curdir + "/" + scriptsPath + "/" + dindScriptName, CmdArgs:[]string{ "up"}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(errors.New("Failed to execute script"))
	}

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

