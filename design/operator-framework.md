# Moving Rook operators to Operator framework

## Background

Rook operators were developed with the help of in ground operator framework called operatorKit. The purpose of the operator kit was to provide a common library for implementing operators. Since the operator kit is not getting much development and with time a more advanced framework for developing operator called operator framework came into the market it would be a nice strategy to leverage the features offered by it.

## Operator framework

The Operator Framework is an open source project that provides developer and runtime Kubernetes tools, enabling you to accelerate the development of an Operator. The Operator Framework includes:

### -> Operator SDK: 
The Operator SDK provides the tools to build, test and package Operators. Initially, the SDK facilitates the marriage of an application’s business logic (for example, how to scale, upgrade, or backup) with the Kubernetes API to execute those operations. Leading practices and code patterns that are shared across Operators are included in the SDK to help prevent reinventing the wheel.


### -> Operator Lifecycle Management: 
The Operator Lifecycle Manager is the backplane that facilitates management of operators on a Kubernetes cluster. With it, administrators can control what Operators are available in what namespaces and who can interact with running Operators. They can also manage the overall lifecycle of Operators and their resources, such as triggering updates to both an Operator and its resources or granting a team access to an Operator for their slice of the cluster.

Benefits of using operator framework can be found here https://coreos.com/blog/introducing-operator-framework.
