# Templating Kubernetes API 

## Background

Currently Rook use strong typed k8s go client to create API objects such as Pod, ReplicaSet, and DaemonSet. 

Strong typed client helps identify coding errors at compilation time, but it is inherently inflexible when we update the objects.

One case is that when switching from ReplicaSet to StatefulSet, all relevant code has to be updated. 

Another issue it is hard to update the objects that have specs not covered in the code. 


## Templating API Object

As other projects embrace template to provide a more flexible way of coding and updating API objects, we should take a look at such approach.

The templating process using `text/template` pkg starts with a template, either coded as a `const` or loaded dynamically from `configmaps`,
supplied with a set of parameters that need to be updated. Depending on needs, certain mapping functions can be defined as well. 

The existing code needs just to pass the parameter and template to create the API object.

### Deployment

Once template is used in the code, we can store the default template in the configmap and allow end users to dynamically adjust. But we should be carefully deny
the template parameters changes.

Objects in Rook Cluster CRD such as resources and security context can be relocated into default template
