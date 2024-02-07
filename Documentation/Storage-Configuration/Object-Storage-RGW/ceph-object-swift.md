---
title: Object Store with Keystone and Swift
---

You can configure the Swift API and the Keystone integration of Ceph RGW natively via the Object Store CRD, which allows native integration of the Rook operated Ceph RGW into [OpenStack](https://www.openstack.org/) clouds.

## Create a Local Object Store with Keystone and Swift

This example will create a `CephObjectStore` that start the RGW service in the cluster providing a Swift API.
Using Swift requires the use of [OpenStack Keystone](https://docs.openstack.org/keystone/latest/) as an authentication provider.

!!! note
This sample requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.

The OSDs must be located on different nodes, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

!!! note
This example assumes that there already is an existing OpenStack Keystone instance ready to use for authentication.

See the [Object Store CRD](../../CRDs/Object-Storage/ceph-object-store-crd.md#object-store-settings), for more detail on the settings (including the Auth section) available for a `CephObjectStore`.

You must set the url in the auth section to point to your keystone service url.

Before you can use keystone as authentication provider you need to have an admin user for rook to access and configure the keystone admin api.

The user credentials for this admin user are provided by a secret in the same namespace which is referenced via the `serviceUserSecretName` property.
The secret contains the credentials with names analogue to the environment variables which would be provided in an OpenStack `openrc` file.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: usersecret
data:
  OS_AUTH_TYPE: cGFzc3dvcmQ=
  OS_IDENTITY_API_VERSION: Mw==
  OS_PASSWORD: c2VjcmV0
  OS_PROJECT_DOMAIN_NAME: RGVmYXVsdA==
  OS_PROJECT_NAME: YWRtaW4=
  OS_USER_DOMAIN_NAME: RGVmYXVsdA==
  OS_USERNAME: YWRtaW4=
type: Opaque
```

Next create the `CephObjectStore` resource:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStore
metadata:
  name: my-store
  namespace: rook-ceph
spec:
  metadataPool:
    failureDomain: host
    replicated:
      size: 3
  dataPool:
    failureDomain: host
    erasureCoded:
      dataChunks: 2
      codingChunks: 1
  auth:
    keystone:
      acceptedRoles:
        - admin
        - member
        - service
      implicitTenants: "swift"
      revocationInterval: 1200
      serviceUserSecretName: usersecret
      tokenCacheSize: 1000
      url: https://keystone.rook-ceph.svc/
  protocols:
    swift:
      accountInUrl: true
      urlPrefix: /swift  
  preservePoolsOnDelete: true
  gateway:
    sslCertificateRef:
    port: 80
    # securePort: 443
    instances: 1
```

After the `CephObjectStore` is created, the Rook operator will then create all the pools and other resources necessary to start the service. This may take a minute to complete.

```console
kubectl create -f object.yaml
```

To confirm the object store is configured, wait for the RGW pod(s) to start:

```console
kubectl -n rook-ceph get pod -l app=rook-ceph-rgw
```

In order to use the object store in Swift using for example the [OpenStack CLI](https://docs.openstack.org/python-openstackclient/latest/), you must create the swift service endpoint in OpenStack/Keystone.
Point the endpoint url to the service endpoint of the created rgw instance.

```sh
openstack service create --name swift object-store
openstack endpoint create --region default --enable swift admin https://rook-ceph-rgw-default.rook-ceph.svc/swift/v1
openstack endpoint create --region default --enable swift internal https://rook-ceph-rgw-default.rook-ceph.svc/swift/v1
```

Afterwards any user which has the rights to access the projects resources (as defined in your OpenStack Keystone instance) can access the object store and create container and objects.
Here the username and project are explicitly set to reflect the user change.

```sh
export OS_USERNAME=alice
export OS_PROJECT=exampleProject
openstack container create exampleContainer
# put /etc/hosts in the new created container
openstack object create exampleContainer /etc/hosts
# retrieve and save the file
openstack object save --file /tmp/hosts.saved exampleContainer /etc/hosts
openstack object delete exampleContainer /etc/hosts
openstack container delete exampleContainer 
```

## Basic concepts

When using Keystone as an authentication provider, Ceph uses the credentials of an admin user (provided in the secret references by `serviceUserSecretName`) to access Keystone.

For each user accessing the object store using Swift, Ceph implicitly creates a user which must be represented in Keystone with an authorized counterpart.
Keystone checks for a user of the same name and based on the name and other parameters ((OpenStack Keystone) project, (OpenStack Keystone) role) allows or disallows access to a swift container or object. 

It is not necessary to create any users in OpenStack Keystone (except for the admin user provided in the `serviceUserSecretName`).

## Keystone setup

To use with rook Keystone must support the v3-API-Version. Other API versions are not supported.

The admin user and all users accessing the Object store must exist and their authorizations configured accordingly in Keystone.

## Openstack setup

To use the Object Store in OpenStack using Swift one must set up the Swift service and create endpoint urls for the Swift service.
See the example in the example configuration "Create a Local Object Store with Keystone and Swift" above for more details and the corresponding CLI calls.