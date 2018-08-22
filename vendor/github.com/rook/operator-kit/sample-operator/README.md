
## Sample Operator
The sample operator creates a custom resources and watches for changes.

### Code Generation
K8s 1.8 requires code generation for all CRDs. See the [code generation sample](code-generation.md) for the steps taken to generate the 
necessary types for this walkthrough. You can skip those details if all you want to do is get the sample running.

### Build
```bash
# from the root of the repo pull all the libraries needed for operator-kit (this may take a while with all the Kubernetes dependencies)
dep ensure

# change directory to sample-operator
cd sample-operator

# build the sample operator binary
CGO_ENABLED=0 GOOS=linux go build

# build the docker container
docker build -t sample-operator:0.1 .
```

### Start the Operator

```bash
# Create the sample operator
$ kubectl create -f sample-operator.yaml

# Wait for the pod status to be Running
$ kubectl get pod -l app=sample-operator
NAME                              READY     STATUS    RESTARTS   AGE
sample-operator-821691060-m5vqp   1/1       Running   0          3m

# View the samples CRD
$ kubectl get crd samples.myproject.io
NAME                   KIND
samples.myproject.io   CustomResourceDefinition.v1beta1.apiextensions.k8s.io
```

### Create the Sample Resource
```bash
# Create the sample
$ kubectl create -f sample-resource.yaml

# See the sample resource
$ kubectl get samples.myproject.io
NAME       KIND
mysample   Sample.v1alpha1.myproject.io
```

### Modify the Sample Resource
Change the value of the `Hello` property in `sample-resource.yaml`, then apply the new yaml.
```bash
kubectl apply -f sample-resource.yaml
```

### Logs

Notice the added and modified Hello= text in the log below

```bash
$ kubectl logs -l app=sample-operator
Getting kubernetes context
Creating the sample resource
Managing the sample resource
Added Sample 'mysample' with Hello=world!
Updated sample 'mysample' from world to goodbye!
```

### Cleanup
```bash
kubectl delete -f sample-resource.yaml
kubectl delete -f sample-operator.yaml
kubectl delete crd samples.myproject.io
```
