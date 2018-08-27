# CRD Code Generation

Starting in Kubernetes 1.8, code can be generated for all CRDs with the same generators that are used for the built-in 
K8s resources. This takes CRDs to the next level of following the same patterns as in-tree resources.
For more background on the code generation, see the [deep dive](https://blog.openshift.com/kubernetes-deep-dive-code-generation-customresources/).

## Directory Structure
First, let's understand the directory structure that is expected with the CRD code generation.
There are two folders at the root level of the `pkg` directory:
- `apis`: Contains types and basic implementation of the CRDs. Some code for the CRD types is generated here.
- `client`: Generated code for strongly typed clients

## Create your types
Before you can generate any code, you will need to put the basic code in place for your CRD. Here are the steps followed
when creating this sample walkthrough.

NOTE: Blank lines are important to the code generator. Until they fix the parsing issues, it will be best if you leave the blank lines in place.

1. Create the `pkg/apis/myproject/v1alpha1` folder.
2. Create [doc.go](pkg/apis/myproject/v1alpha1/register.go) with the following contents. Notice the instructions for the code generator.
```go
// +k8s:deepcopy-gen=package,register

// Package v1 is the v1 version of the API.
// +groupName=myproject
package v1alpha1
```
3.  Create [types.go](pkg/apis/myproject/v1alpha1/types.go) with the definition for your types. Each type will need the code generation attributes.
```go
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Sample struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              SampleSpec `json:"spec"`
}
```
4. Create [register.go](pkg/apis/myproject/v1alpha1/register.go) with the CRD definition that will register your CRD type. No code generation attributes are needed.

## Run the code generator
During a build process, it is recommended to run the code generator to make sure you always have the latest fixes for the code generation.
For this sample, a script [codegen.sh](codegen.sh) is provided to simplify the call to the k8s code generation script that is found under the `vendor` folder.
```bash
# Run the code generator
./codegen.sh
```

This script will generate the following artifacts:
- `apis/myproject/v1alpha1/zz_generated.deepcopy.go`: The deep copy methods for the CRD types necessary in K8s 1.8
- `client/clientset/versioned`: Many files are generated under this folder for access by a strongly typed client.