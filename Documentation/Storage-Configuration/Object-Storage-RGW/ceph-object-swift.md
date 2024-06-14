---
title: Object Store with Keystone and Swift
---

!!! note
    The Object Store with Keystone and Swift is currently in experimental mode.

Ceph RGW can integrate natively with the Swift API and Keystone via the CephObjectStore CRD. This allows native integration of Rook-operated Ceph RGWs into [OpenStack](https://www.openstack.org/) clouds.

!!! note
    Authentication via the OBC and COSI features is not affected by this configuration.

## Create a Local Object Store with Keystone and Swift

This example will create a `CephObjectStore` that starts the RGW service in the cluster providing a Swift API.
Using Swift requires the use of [OpenStack Keystone](https://docs.openstack.org/keystone/latest/) as an authentication provider.

The OSDs must be located on different nodes, because the [`failureDomain`](../../CRDs/Block-Storage/ceph-block-pool-crd.md#spec) is set to `host` and the `erasureCoded` chunk settings require at least 3 different OSDs (2 `dataChunks` + 1 `codingChunks`).

More details on the settings available for a `CephObjectStore` (including the `Auth` section) can be found in the [Object Store CRD](../../CRDs/Object-Storage/ceph-object-store-crd.md#object-store-settings) document.

Set the url in the auth section to point to the keystone service url.

Prior to using keystone as authentication provider an admin user for rook to access and configure the keystone admin api is required.

The user credentials for this admin user are provided by a secret in the same namespace which is referenced via the `serviceUserSecretName` property.
The secret contains the credentials with names analogue to the environment variables used in an OpenStack `openrc` file.

!!! note
    This example requires *at least 3 bluestore OSDs*, with each OSD located on a *different node*.
    This example assumes an existing OpenStack Keystone instance ready to use for authentication.

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
    # note that s3 is enabled by default if protocols.s3.enabled is not explicitly set to false
  preservePoolsOnDelete: true
  gateway:
    sslCertificateRef:
    port: 80
    # securePort: 443
    instances: 1
```

After the `CephObjectStore` is created, the Rook operator will create all the pools and other resources necessary to start the service. This may take a minute to complete.

```console
kubectl create -f object.yaml
```

The start of the RGW pod(s) confirms that the object store is configured.

```console
kubectl -n rook-ceph get pod -l app=rook-ceph-rgw
```

The swift service endpoint in OpenStack/Keystone must be created, in order to use the object store in Swift using for example the [OpenStack CLI](https://docs.openstack.org/python-openstackclient/latest/).
The endpoint url should be set to the service endpoint of the created rgw instance.

```sh
openstack service create --name swift object-store
openstack endpoint create --region default --enable swift admin https://rook-ceph-rgw-default.rook-ceph.svc/swift/v1
openstack endpoint create --region default --enable swift internal https://rook-ceph-rgw-default.rook-ceph.svc/swift/v1
```

Afterwards any user which has the rights to access the projects resources (as defined in the OpenStack Keystone instance) can access the object store and create container and objects.
Here the username and project are explicitly set to reflect use of the (non-admin) user.

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
Keystone checks for a user of the same name. Based on the name and other parameters ((OpenStack Keystone) project, (OpenStack Keystone) role) Keystone allows or disallows access to a swift container or object. Note that the implicitly created users are creaded in addition to any users that are created through other means, so Keystone authentication is not exclusive.

It is not necessary to create any users in OpenStack Keystone (except for the admin user provided in the `serviceUserSecretName`).

## Keystone setup

Keystone must support the v3-API-Version to be used with Rook. Other API versions are not supported.

The admin user and all users accessing the Object store must exist and their authorizations configured accordingly in Keystone.

## Openstack setup

To use the Object Store in OpenStack using Swift the Swift service must be set and the endpoint urls for the Swift service created.
The example configuration "Create a Local Object Store with Keystone and Swift" above contains more details and the corresponding CLI calls.
