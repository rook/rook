♜ [Rook NooBaa Design](README.md) /
# NooBaaSystem

NooBaaSystem is a CRD that represents a single installation of NooBaa that includes a set of sub-resources (backing-stores, bucket-classes, and buckets) and has a lifecycle as a single integrated system.


# Definitions

- CRD: [noobaa-system-crd.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-system-crd.yaml)
- Example: [noobaa-system-example.yaml](../../cluster/examples/kubernetes/noobaa/noobaa-system-example.yaml)


# Reconcile

The operator watches for NooBaaSystem changes and reconcile them to apply the following deployment:

- Kubernetes Resources
  - General:
    - ServiceAccount
    - Role
    - RoleBinding
  - Core server:
    - StatefulSet
    - Service (mgmt)
    - Service (s3)
    - Route/Ingress
  - Endpoints:
    - Deployment
    - HorizontalPodAutoscaler
- NooBaa Setup
  - Admin Account
    - Once the server is up and running the operator will call an API to setup a new system in the server which returns a secret token:
      - `noobaa-admin-mgmt-secret` will be created to contain the admin account token. It will be used for next API calls on this system.
      - `noobaa-admin-s3-secret` will be created with the admin S3 credentials to allow easy onboarding for the admin.
  - Internal backing-store
    - In order to reduce the friction of setting up backing-stores right after deployment, we create an internal backing-store with the following characteristics:
      - The internal store has limited size
      - It is using the core server PV for storage
      - It should not be used for production workloads.
  - Default bucket-class
    - This is a simple class that uses just the internal backing-store.
    - Once backing-stores are added the default class should be updated and existing data will automatically move from internal store to the new stores.
  - first.bucket
    - The operator will create a `first.bucket` using the default bucket-class.


# Read Status

The operator will set the status of the NooBaaSystem to represent the current state of reconciling to the desired state.\
Here is the example status structure as would be returned by a `kubectl get noobaa/noobaa-system -n rook-noobaa -o yaml`:

```yaml
apiVersion: noobaa.rook.io/v1alpha1
kind: NooBaaSystem
metadata:
  name: noobaa-system
  namespace: rook-noobaa
spec:
    # ...
status:

  health:
    backingStores: OK
    bucketClasses: OK
    buckets: OK
    accounts: OK
    issues: []

  counters:
    backingStores: 2
    bucketClasses: 3
    buckets: 33
    accounts: 3
  
  readme: |

Welcome to NooBaa

S3 Endpoint
-----------
- Access key            : export AWS_ACCESS_KEY_ID=$(kubectl get secret noobaa-admin-s3-secret -n rook-noobaa -o json | jq -r '.data.AWS_ACCESS_KEY_ID|@base64d')
- Secret key            : export AWS_SECRET_ACCESS_KEY=$(kubectl get secret noobaa-admin-s3-secret -n rook-noobaa -o json | jq -r '.data.AWS_SECRET_ACCESS_KEY|@base64d')
- External address      : https://222.222.222.222:8443
- ClusterIP address     : https://s3.noobaa
- NodePort address      : http://192.168.99.100:30361
- Port forwarding       : kubectl port-forward -n noobaa service/s3 10443:443 # then open https://localhost:10443
- aws-cli               : alias s3="aws --endpoint https://localhost:10443 s3"

Management
-------------
- Username/password     : kubectl get secret noobaa-admin-mgmt-secret -n rook-noobaa -o json | jq '.data|map_values(@base64d)'
- External address      : https://111.111.111.111:8443
- ClusterIP address     : https://noobaa-mgmt.noobaa:8443
- Node port address     : http://192.168.99.100:30785
- Port forwarding       : kubectl port-forward -n noobaa service/noobaa-mgmt 11443:8443 # then open https://localhost:11443
```

Example health status when there is an issue with the availability of a backing-store:
```yaml
status:
  health:
    backingStores: WARNING
    bucketClasses: OK
    buckets: OK
    accounts: OK
    issues:
      - title: backingStore "aws" is not accessible
        createTime: "2019-06-04T13:05:35.473Z"
        lastTime: "2019-06-04T13:05:35.473Z"
```


# Delete

The operator will detect deletion of a system CR, and will followup by deleting all the owned resources.

In general we can leave this to Garbage Collection as described here:

https://kubernetes.io/docs/concepts/workloads/controllers/garbage-collection/

While this works, and there are no known dependencies on the order to deletions, we will prefer to call it explicitly to handle finalizers faster, and leave the GC to edge cases.
