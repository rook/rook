
## Kubernetes Operator Kit
A Kubernetes Operator is a controller for custom resources. The purpose of the operator kit is to provide a common
library for implementing operators. 

The library originated from the Rook operator. Much more thought needs to be put into API design, but at least provides the basis for the library with working code.
With enough interest, this could become a Kubernetes incubator project.

### Features
The operator kit is a simple collection of features that will be useful for operators.
- **CRD handling**: creating, retrieving, and watching CRDs on K8s 1.7+
- **TPR handling**: creating, retrieving, and watching TPRs on versions prior to 1.7
- **Timing**: helpers to timeout when taking too long or retry when when working with kubernetes resources


### Roadmap 
The operator kit is still in its infancy and needs plenty of work before it is considered stable. 
- Community collaboration on the requirements and design
- Leader election for HA
- Tests

The conversation has been started [here](https://docs.google.com/document/d/1NJhFcNezJyLM952eaYVcdfIQFQYWsAx4oTaA82-Frdk).

## Sample Code
To help you get started, a simple operator with a single custom resource is provided [here](sample-operator/README.md).

## Contributing

Contributions are welcome! See [Contributing](CONTRIBUTING.md) to get started.

## Report a Bug

For filing bugs, suggesting improvements, or requesting new features, help out by opening an [issue](https://github.com/rook/operator-kit/issues).

## Licensing

The operator kit is under the Apache 2.0 license. The appropriate license information can be found in the headers of the source files.
