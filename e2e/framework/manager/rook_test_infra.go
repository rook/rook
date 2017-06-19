package rook_test_infra

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"

	"os"
	"path/filepath"

	"github.com/rook/rook/e2e/framework/contracts"
	"github.com/rook/rook/e2e/framework/enums"
	"github.com/rook/rook/e2e/framework/objects"
	"github.com/rook/rook/e2e/framework/transport"
	"github.com/rook/rook/e2e/framework/utils"
)

type rookTestInfraManager struct {
	transportClient contracts.ITransportClient
	platformType    enums.RookPlatformType
	dockerized      bool
	dockerContext   *objects.DockerContext
	k8sVersion      enums.K8sVersion
	isSetup         bool
	isRookInstalled bool
}

var (
	r      *rookTestInfraManager
	once   sync.Once
	curdir string
)

const (
	tempImageFileName              = "temp_image.tar"
	rookOperatorFileName           = "rook-operator.yaml"
	rookClusterFileName            = "rook-cluster.yaml"
	rookToolsFileName              = "rook-tools.yaml"
	podSpecPath                    = "src/github.com/rook/rook/demo/kubernetes"
	scriptsPath                    = "scripts"
	k8sFalsePostiveSuccessErrorMsg = "exit status 1" //When kubectl drain is executed, exit status 1 is always returned in stdout
	rookDindK8sClusterScriptv1_5   = "rook-dind-cluster-v1.5.sh"
	rookDindK8sClusterScriptv1_6   = "rook-dind-cluster-v1.6.sh"
	rookOperatorImagePodSpecTag    = "quay.io/rook/rookd:master-latest"
	rookToolsImagePodSpecTag       = "quay.io/rook/toolbox:master-latest"
	rookOperatorPrefix             = "quay.io/rook/rookd"
	rookToolboxPrefix              = "quay.io/rook/toolbox-amd64"
	rookOperatorCreatedTpr         = "cluster.rook.io"
	masterContinerName             = "kube-master"
	node1ContinerName              = "kube-node-1"
	node2ContinerName              = "kube-node-2"
	cephBaseImageName              = "ceph/base"
	defaultTagName                 = "master-latest"
)

func getPodSpecPath() string {
	return filepath.Join(os.Getenv("GOPATH"), podSpecPath)
}

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
		return fmt.Errorf("Unsupported Rook Platform Type: " + platformType.String()), r
	default:
		return fmt.Errorf("Unsupported Rook Platform Type: " + platformType.String()), r
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

//Gets the platform the manager was configured with when created
func (r *rookTestInfraManager) GetRookPlatform() enums.RookPlatformType {
	return r.platformType
}

//Wrapper method for executing docker commands
//if method fails it will panic
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

	if err != nil {
		panic(err)
	}

	fmt.Println("Command succeeded...")

	return stdOut, stdErr
}

//Get the k8s cluster creation script based on k8s version desired
func getDindScriptName(k8sVersion enums.K8sVersion) (dindScriptName string) {
	switch k8sVersion {
	case enums.V1dot5:
		dindScriptName = rookDindK8sClusterScriptv1_5
	case enums.V1dot6:
		dindScriptName = rookDindK8sClusterScriptv1_6
	default:
		panic(fmt.Errorf("Unsupported Kubernetes version: " + k8sVersion.String()))
	}

	return dindScriptName
}

func (r *rookTestInfraManager) copyImageToNode(containerId string, imageName string) error {
	fmt.Println("looking in local repository for the docker image: " + imageName)
	cmdOut := utils.ExecuteCommand(objects.CommandArgs{Command: "docker", CmdArgs: []string{"images", "-q", imageName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		fmt.Println("Image does not exist locally")
	}

	cmdOut = utils.ExecuteCommand(objects.CommandArgs{Command: "docker", CmdArgs: []string{"images"}})
	fmt.Printf("Did not find docker images %s locally\nDocker images:\n%s\n", imageName, cmdOut.StdOut)

	if strings.EqualFold(cmdOut.StdOut, "") {
		fmt.Println("searching remotely for the docker image: " + imageName)
		cmdOut := utils.ExecuteCommand(objects.CommandArgs{Command: "docker", CmdArgs: []string{"pull", imageName}})

		if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
			panic(fmt.Errorf("Failed to pull image : %s", imageName))
		}
		fmt.Println("success...")
	}

	fmt.Println("archiving the docker image: " + tempImageFileName)
	cmdOut = utils.ExecuteCommand(objects.CommandArgs{Command: "docker", CmdArgs: []string{"save", "-o", tempImageFileName, imageName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to archive image : %s", imageName))
	}
	fmt.Println("success...")

	fmt.Println("copying archived image to node container")
	_, _ = r.executeDockerCommand("", enums.Copy, tempImageFileName, containerId+":/"+tempImageFileName)
	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to copy image to container : %s", imageName))
	}
	fmt.Println("success...")

	fmt.Println("importing archived image into container registry")
	_, _ = r.executeDockerCommand(containerId, enums.Exec, "/bin/bash", "-c", "docker load -i /"+tempImageFileName)
	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed load image into containers repository : %s", imageName))
	}
	fmt.Println("success...")

	return nil
}

//This method stands up a k8s cluster and pre-configures it for running tests
func (r *rookTestInfraManager) ValidateAndSetupTestPlatform(skipInstall bool) {

	if skipInstall {
		return
	}
	if r.isSetup {
		return
	}
	r.TearDownInfrastructureCreatedEnvironment()

	fmt.Println("pulling the docker image: " + cephBaseImageName)
	cmdOut := utils.ExecuteCommand(objects.CommandArgs{Command: "docker", CmdArgs: []string{"pull", cephBaseImageName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to pull image"))
	}
	fmt.Println("success...")

	dindScriptName := getDindScriptName(r.k8sVersion)

	//make the k8s creation script executable
	cmdOut = utils.ExecuteCommand(objects.CommandArgs{Command: "chmod", CmdArgs: []string{"+x", curdir + "/" + scriptsPath + "/" + dindScriptName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to chmod cluster creation script"))
	}

	//launch the k8s creation script
	cmdOut = utils.ExecuteCommand(objects.CommandArgs{Command: curdir + "/" + scriptsPath + "/" + dindScriptName, CmdArgs: []string{"up"}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to execute cluster creation execute script"))
	}

	//Untaint all nodes making them schedulable
	k8sClient := transport.CreateNewk8sTransportClient()

	_, _, err := k8sClient.ExecuteCmd([]string{"taint", "nodes", "--all", "node-role.kubernetes.io/master-"})

	if err != nil && !strings.EqualFold(err.Error(), k8sFalsePostiveSuccessErrorMsg) {
		panic(err)
	}

	//if any of these methods fail, the executeDockerCommand will panic
	k8sMasterContainerId := r.getContainerIdByName(masterContinerName)
	k8sNode1ContainerId := r.getContainerIdByName(node1ContinerName)
	k8sNode2ContainerId := r.getContainerIdByName(node2ContinerName)

	//install rbd shim
	_, _ = r.executeDockerCommand("", enums.Copy, curdir+"/"+scriptsPath+"/rbd", k8sMasterContainerId+":/bin/rbd")
	_, _ = r.executeDockerCommand(k8sMasterContainerId, enums.Exec, "chmod", "+x", "/bin/rbd")

	_, _ = r.executeDockerCommand("", enums.Copy, curdir+"/"+scriptsPath+"/rbd", k8sNode1ContainerId+":/bin/rbd")
	_, _ = r.executeDockerCommand(k8sNode1ContainerId, enums.Exec, "chmod", "+x", "/bin/rbd")

	_, _ = r.executeDockerCommand("", enums.Copy, curdir+"/"+scriptsPath+"/rbd", k8sNode2ContainerId+":/bin/rbd")
	_, _ = r.executeDockerCommand(k8sNode2ContainerId, enums.Exec, "chmod", "+x", "/bin/rbd")

	r.isSetup = true
}

func (r *rookTestInfraManager) getContainerIdByName(containerName string) (containerId string) {
	_, containerId = r.executeDockerCommand("", enums.Ps, "--filter", "name="+containerName, "--format", "{{.ID}}")

	return containerId
}

//method for create rook-operator via kubectl
func createK8sRookOperator(k8sHelper *utils.K8sHelper, tag string) error {

	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(), rookOperatorFileName))

	if err != nil {
		return err
	}
	if !bytes.Contains(raw, []byte(rookOperatorImagePodSpecTag)) {
		return fmt.Errorf(" %s tag to be replaced with %s couldn't be found in rook-operator.yaml", rookOperatorImagePodSpecTag, tag)
	}
	rawUpdated := bytes.Replace(raw, []byte(rookOperatorImagePodSpecTag), []byte(tag), 1)
	rookOperator := string(rawUpdated)

	fmt.Println("Creating rook-operator with tag of: " + tag)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookOperator)

	if exitCode != 0 {
		return fmt.Errorf(string("Failed to create rook-operator pod; kubectl exit code = " + string(exitCode)))
	}

	fmt.Println()
	if !k8sHelper.IsThirdPartyResourcePresent(rookOperatorCreatedTpr) {
		return fmt.Errorf("Failed to start Rook Operator; k8s thirdpartyresource did not appear")
	} else {
		fmt.Println("Rook Operator started")
	}

	return nil
}

func createK8sRookToolbox(k8sHelper *utils.K8sHelper, tag string) (err error) {

	//Create rook client
	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(), rookToolsFileName))

	if err != nil {
		panic(err)
	}

	if !bytes.Contains(raw, []byte(rookToolsImagePodSpecTag)) {
		return fmt.Errorf(" %s tag to be replaced with %s couldn't be found in rook-tools.yaml", rookToolsImagePodSpecTag, tag)
	}

	rawUpdated := bytes.Replace(raw, []byte(rookToolsImagePodSpecTag), []byte(tag), 1)
	rookClient := string(rawUpdated)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookClient)

	if exitCode != 0 {
		return fmt.Errorf(string(exitCode))
	}

	if !k8sHelper.IsPodRunningInNamespace("rook-tools") {
		return fmt.Errorf("Rook Toolbox couldn't start")
	} else {
		fmt.Println("Rook Toolbox started")
	}

	return nil
}

func createk8sRookCluster(k8sHelper *utils.K8sHelper, tag string) error {

	raw, err := ioutil.ReadFile(path.Join(getPodSpecPath(), rookClusterFileName))

	if err != nil {
		return err
	}
	if !bytes.Contains(raw, []byte(defaultTagName)) {
		return fmt.Errorf(" %s tag to be replaced with  %s couldn't be found in rook-cluster.yaml", defaultTagName, tag)
	}
	rawUpdated := bytes.Replace(raw, []byte(defaultTagName), []byte(tag), 1)
	rookCluster := string(rawUpdated)

	_, _, exitCode := r.transportClient.CreateWithStdin(rookCluster)

	if exitCode != 0 {
		return fmt.Errorf("Failed to create rook-cluster pod; kubectl exit code = " + string(exitCode))
	}

	if !k8sHelper.IsServiceUpInNameSpace("rook-api") {
		fmt.Println("Rook Cluster couldn't start")
	} else {
		fmt.Println("Rook Cluster started")
	}

	return nil
}

func (r *rookTestInfraManager) InstallRook(tag string, skipInstall bool) (err error) {
	if skipInstall {
		return
	}
	if r.isRookInstalled {
		return
	}

	rookOperatorTag := rookOperatorPrefix + ":" + tag
	rookToolboxTag := rookToolboxPrefix + ":" + tag

	err = r.copyImageToNode(r.getContainerIdByName(masterContinerName), rookOperatorTag)
	if err != nil {
		panic(err)
	}

	err = r.copyImageToNode(r.getContainerIdByName(node1ContinerName), rookOperatorTag)
	if err != nil {
		panic(err)
	}

	err = r.copyImageToNode(r.getContainerIdByName(node2ContinerName), rookOperatorTag)
	if err != nil {
		panic(err)
	}
	err = r.copyImageToNode(r.getContainerIdByName(masterContinerName), rookToolboxTag)
	if err != nil {
		panic(err)
	}

	err = r.copyImageToNode(r.getContainerIdByName(node1ContinerName), rookToolboxTag)
	if err != nil {
		panic(err)
	}

	err = r.copyImageToNode(r.getContainerIdByName(node2ContinerName), rookToolboxTag)
	if err != nil {
		panic(err)
	}

	//Create rook operator
	k8sHelp := utils.CreatK8sHelper()

	err = createK8sRookOperator(k8sHelp, rookOperatorTag)
	if err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Second) ///TODO: add real check here

	//Create rook cluster
	err = createk8sRookCluster(k8sHelp, tag)
	if err != nil {
		panic(err)
	}

	time.Sleep(5 * time.Second)

	//Create rook client
	err = createK8sRookToolbox(k8sHelp, rookToolboxTag)

	if err != nil {
		panic(err)
	}

	r.isRookInstalled = true

	return nil
}

func (r *rookTestInfraManager) isContainerRunning(containerId string) bool {
	dockerClient := r.dockerContext.Get_DockerClient()

	_, stdErr, _ := dockerClient.ExecuteCmd([]string{"ps", "--filter", "status=running", "--filter", "id=" + containerId, "--format", "\"{{.ID}}\""})

	return strings.EqualFold(stdErr, containerId)
}

func (r rookTestInfraManager) TearDownInfrastructureCreatedEnvironment() error {

	dindScriptName := getDindScriptName(r.k8sVersion)

	cmdOut := utils.ExecuteCommand(objects.CommandArgs{Command: "chmod", CmdArgs: []string{"+x", curdir + "/" + scriptsPath + "/" + dindScriptName}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to chmod script"))
	}

	cmdOut = utils.ExecuteCommand(objects.CommandArgs{Command: curdir + "/" + scriptsPath + "/" + dindScriptName, CmdArgs: []string{"clean"}})

	if cmdOut.Err != nil || cmdOut.ExitCode != 0 {
		panic(fmt.Errorf("Failed to execute clean up script"))
	}

	r.isSetup = false
	r.isRookInstalled = false

	return nil
}
