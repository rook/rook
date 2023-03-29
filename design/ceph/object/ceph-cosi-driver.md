# Ceph COSI Driver Support

## Targeted for v1.12

## Background

Container Object Storage Interface (COSI) is a specification for container orchestration frameworks to manage object storage. Even though there is no standard protocol defined for Object Store, it has flexibility to add support for all. The COSI spec abstracts common storage features such as create/delete buckets, grant/revoke access to buckets, attach/detach buckets, and more. COSI is released v1alpha1 with Kubernetes 1.25.
More details about COSI can be found [here](https://kubernetes.io/blog/2022/09/02/cosi-kubernetes-object-storage-management/).
It is projected that COSI will be the only supported object storage driver in the future and OBCs will be deprecated after some time.

## COSI Driver Deployment

The [COSI controller](https://github.com/kubernetes-sigs/container-object-storage-interface-controller) is deployed as container in the default namespace. It is not deployed by Rook.

The Ceph COSI driver is deployed as a deployment with a [COSI sidecar container](https://github.com/kubernetes-sigs/container-object-storage-interface-provisioner-sidecar). The Ceph COSI driver is deployed in the same namespace as the Rook operator. The Ceph COSI driver is deployed with a service account that has the following RBAC permissions:

## Integration plan with Rook

The aim to support the v1alpha1 version of COSI in Rook v1.12. It will be extended to beta and release versions as appropriate. There will be one COSI Driver support from Rook. The driver will be started automatically with default settings when first Object Store gets created. The driver will be deleted when Rook Operator is uninstalled. The driver will be deployed in the same namespace as Rook operator. The user can provide additional setting via new `CephCOSIDriver` CRD which is owned by Rook.

### Pre-requisites

- COSI CRDs should be installed in the cluster via following command

```bash
kubectl apply -k github.com/kubernetes-sigs/container-object-storage-interface-api
```

- COSI controller should be deployed in the cluster via following command

```bash
kubectl apply -k github.com/kubernetes-sigs/container-object-storage-interface-controller
```

### End to End Workflow

#### Changes in Common.yaml

Following contents need to be append to `common.yaml` :

- <https://github.com/ceph/ceph-cosi/blob/master/resources/sa.yaml>
- <https://github.com/ceph/ceph-cosi/blob/master/resources/rbac.yaml>

#### ceph-object-cosi-controller

The `ceph-object-cosi-controller` will start the Ceph COSI Driver pod with default settings on the Rook Operator Namespace. The controller will bring up the Ceph COSI driver when first object store is created and will stop the COSI driver when Rook Operator is uninstalled only if it detect COSI Controller is running on default namespace. The controller will also watch for CephCosiDriver CRD, if defined the driver will be started with the settings provided in the CRD. If the Ceph COSI driver if up and running,it  will also create `CephObjectStoreUser` named  `cosi` for each object store which internally creates a secret rook-ceph-object-user-<objectstore-name>-cosi provides credentials for the object store. This can be specified in the BucketClass and BucketAccessClass. Also this controller ensures maximum one CephCosiDriver CRD exists in the cluster. For v1.12 the Ceph COSI Driver will be supported only for single site CephObjectStore aka object stores not configured with multisite settings like zone/zonegroup/realm.

#### cephcosidriver.ceph.rook.io CRD

The users can define following CRD so that configuration related the Ceph COSI driver can be passed. This is not mandatory and the driver will be started with default settings if this CRD is not defined.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCosiDriver
metadata:
  name: rook-ceph-cosi
  namespace: rook-ceph
spec:
  deploymentStrategy: "Auto"
  placement:
  #  nodeAffinity:
  #  podAffinity:
  #  tolerations:
  resource:
  #  requests:
  #    cpu: "100m"
  #    memory: "128Mi"
  #  limits:
  #    cpu: "100m"
  #    memory: "128Mi"
```

The `deploymentStrategy` can be `Auto` or `Force` or `Disable`. If `Auto` is specified the driver will be started automatically when first object store is created and will be stopped when last object store is deleted. If `Force` is specified the driver will be started when the CRD is created and will be stopped when the CRD is deleted. The user can also disable the driver by specifying `Disable` as the value. The `placement` field can be used to specify the placement settings for the driver. The `resource` field can be used to specify the resource requirements for the driver. Both can refer from [CephClusterCRD Documentation](/Documentation/CRDs/Cluster/ceph-cluster-crd.md).

#### Creating COSI Related CRDs

There are five different kubernetes resources related to COSI. They are `Bucket`, `BucketAccess`, `BucketAccessClass`, `BucketClass` and `BucketClaim`. The user can create these resources using following `kubectl` command. All the examples will be added to deploy/examples/cosi directory in the Rook repository.

```bash
kubectl create -f deploy/examples/cosi/bucketclass.yaml
kubectl create -f deploy/examples/cosi/bucketclaim.yaml
kubectl create -f deploy/examples/cosi/bucketaccessclass.yaml
kubectl create -f deploy/examples/cosi/bucketaccess.yaml
```

User need refer Secret which contains access credentials for the object store in the `Parameter`  field of `BucketClass` and `BucketAccessClass` CRD as below:

```yaml
Parameter
  objectUserSecretName: "rook-ceph-object-user-<objectstore-name>-cosi"
  objectStoreNamespace: "<objectstore-store-namespace>"
```

#### Consuming COSI Bucket

The user needs to mount the secret as volume created by `BucketAccess` to the application pod. The user can use the secret to access the bucket by parsing the mounted file.

```yaml
spec:
  containers:
      volumeMounts:
        - name: cosi-secrets
          mountPath: /data/cosi
  volumes:
  - name: cosi-secrets
    secret:
      secretName: ba-secret
```

```bash
# cat /data/cosi/bucket_info.json
```

```json
{
      apiVersion: "v1alpha1",
      kind: "BucketInfo",
      metadata: {
          name: "ba-$uuid"
      },
      spec: {
          bucketName: "ba-$uuid",
          authenticationType: "KEY",
          endpoint: "https://rook-ceph-my-store:443",
          accessKeyID: "AKIAIOSFODNN7EXAMPLE",
          accessSecretKey: "wJalrXUtnFEMI/K...",
          region: "us-east-1",
          protocols: [
            "s3"
          ]
      }
    }
```

#### Coexistence of COSI and lib-bucket-provisioner

Currently the ceph object store provisioned via Object Bucket Claim (OBC). They both can coexist and can even use same backend bucket from ceph storage. No deployment/configuration changes are required to support both. The lib-bucket-provisioner is deprecated and eventually will be replaced by COSI when it becomes more and more stable. The CRDs used by both are different hence there is no conflicts between them. The user can create OBC and COSI BucketClaim for same backend bucket which have always result in conflicts. Even though credentials for access the buckets are different, both have equal permissions on accessing the bucket. If Rooks creates OBC with `Delete` reclaim policy and same backend bucket is used by COSI BucketClaim with same policy, then bucket will be deleted when either of them is removed.

#### Transition from lib-bucket-provisioner to COSI

This applied to OBC with reclaim policy is `Retain` otherwise the bucket will be deleted when OBC is deleted. So no point in migrating the OBC with `Delete` reclaim policy.

- First the user need to create a **COSI Bucket resource** pointing to the backend bucket.
- Then user can create BucketAccessClass and BucketAccess using the COSI Bucket CRD.
- Now the update application's credentials with BucketAccess secret, for OBC it was combination of secret and config map with keys word like AccessKey, SecretKey, Bucket, BucketHost etc. Here details in as JSON format in the [secret](#consuming-cosi-bucket).
- Finally the user need to delete the existing OBC.

### Points to remember

#### Ceph COSI Driver Requirements

- The credentials/endpoint for the CephObjectStore should be available by creating CephObjectStoreUser with proper permissions
- The COSI controller should be deployed in the cluster
- Rook can manage one Ceph COSI driver per Rook operator
- Rook should not modify COSI resources like Bucket, BucketAccess, BucketAccessClass, or BucketClass.

#### Rook Requirements

##### Current

- Users should able to manage both OBC and COSI Bucket/BucketAccess resources with Same Rook Operator.
- When provisioning ceph COSI driver Rook must uniquely identify the driver name like <namespace of rook operator>-cosi-driver so that multiple COSI drivers or multiple Rook instances within a Kubernetes cluster will not collide.

##### Future

- Rook needs to dynamically create/update the secret containing the credentials of the ceph object store for the ceph COSI driver when user creates/updates the CephObjectStoreUser keys.
