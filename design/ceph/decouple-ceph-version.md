# Decoupling the Ceph version

**Targeted for v0.9**

Today the version of Ceph is tied to the version of Rook. Each release of Rook releases a specific version of Ceph that is embedded in the same docker image.
This needs to be changed such that the version of Ceph is decoupled from the release of Rook. By separating the decision of which version of Ceph will be deployed with Rook, we have a number of advantages:
- Admins can choose to run the version of Ceph that meets their requirements.
- Admins can control when they upgrade the version of Ceph. The data path upgrade needs to be carefully controlled by admins in production environments.
- Developers can test against any version of Ceph, whether a stable version of Luminous or Mimic, or even a private dev build.

Today Rook still includes Luminous, even while Mimic was released several months ago. A frequently asked question from users is when we are going to update to Mimic so they can take advantage of the new features such as the improved dashboard. That question will not be heard anymore after this design change. As soon as a new build of Ceph is available, Rook users will be able to try it out.

## Coupled (Legacy) Design

The approach of embedding Ceph into the Rook image had several advantages that contributed to the design.
- Simpler development and test matrix. A consistent version of Ceph is managed and there are no unstable Ceph bits running in the cluster.
- Simpler upgrade path. There is only one version to worry about upgrading.

The project is growing out of these requirements and we need to support some added complexity in order to get the benefits of the decoupled versions.

## New Design

There are two versions that will be specified independently in the cluster: the Rook version and the Ceph version.

### Rook Version

The Rook version is defined by the operator's container `image` tag. All Rook containers launched by the operator will also launch the same version of the Rook image.
The full image name is an important part of the version. This allows the container to be loaded from a private repo if desired.

In this example, the Rook version is `rook/ceph:v0.8.1`.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rook-ceph-operator
spec:
  template:
    spec:
      containers:
      - name: rook-ceph-operator
        image: rook/ceph:v0.8.1
```

### Ceph Version

The Ceph version is defined under the property `cephVersion` in the Cluster CRD. All Ceph daemon containers launched by the Rook operator will use this image, including the mon, mgr,
osd, rgw, and mds pods. The significance of this approach is that the Rook binary is not included in the daemon containers. All initialization performed by Rook to generate the Ceph config and prepare the daemons must be completed in an [init container](https://github.com/rook/rook/issues/2003). Once the Rook init containers complete their execution, the daemon container will run the Ceph image. The daemon container will no longer have Rook running.

In the following Cluster CRD example, the Ceph version is Mimic `13.2.2` built on 23 Oct 2018.

```yaml
apiVersion: ceph.rook.io/v1
kind: Cluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ceph/ceph:v13.2.2-20181023
```

### Operator Requirements

The operator needs to run the Ceph client tools to manage the cluster. For example, the `ceph` tool is needed for general Ceph configuration and status, while `radosgw-admin` is required for managing an object store. Therefore, all the necessary client tools will still be included in the Rook image.

The client tools are tested by the Ceph team to be backward and forward compatible by two versions. This means the operator can support a version of Ceph up to two versions older than the client tools it contains.
With each Rook release, the tools will be included from the latest release of Ceph. For example, in 0.9 Rook will likely include the Mimic tools. Upgrades would be supported from Luminous to Mimic.
Rook 0.9 can also be tested to support upgrades to Nautilus since they may be released in the same time frame. Since the Ceph tools are forward compatible, the Mimic tools will be sufficient to support upgrading to Nautilus.
If Nautilus is released after Rook 0.9, a patch release can be made to 0.9 so that Rook can officially support the upgrade at that point. The changes in the patch release should be minimal since upgrading to Nautilus could have been mostly planned for in 0.9.

The operator will be made to understand differences in the Ceph versions that are necessary for orchestration. Some examples might include:
- If running Luminous, start the Ceph dashboard on http. If running Mimic, a self-signed cert could be generated to start the dashboard with https.
- If a new daemon is added in a future Ceph release, the operator would understand to deploy that daemon only if the Ceph version is at least that version.

### Supported Versions

Rook will support a very specific list of major versions. Outside these versions, Rook will not be aware of the needs for configuring and upgrading the cluster.
In v0.9, the supported versions will be:
- luminous (ceph/ceph:v12.2.x)
- mimic (ceph/ceph:v13.2.x)

Depending on the timing of the 0.9 and Nautilus releases, Nautilus will likely be supported either in 0.9 or a patch release. Versions not yet officially supported
can be tested with settings in the CRD to be mentioned below.

All Rook implementation specific to a Ceph version will apply to all patch releases of that major release. For example, Rook is not expected to have any differences handling
various Mimic patch releases.

### Upgrades

The flexibility during upgrades will now be improved since the upgrade of Rook will be independent from the upgrade to the Ceph version.
- To upgrade Rook, update the version of the Rook operator container
- To upgrade Ceph, make sure Rook is running the latest release, then update the `cephVersion.image` in the cluster CRD

The versions to be supported during upgrade will be a specific set for each version of Rook. In 0.9, it is anticipated that the only upgrade of Ceph
supported would only be Luminous to Mimic. When Rook officially adds support for a release of Ceph (ie. Nautilus), the upgrade path will also be supported from one previous version.
For example, after Nautilus support is added, Luminous users would first need to upgrade to Mimic and then Nautilus. While it may be possible to skip versions
during upgrade, it is not supported in order to keep the testing more scoped.

#### Upgrade Sequence

Each time the operator starts, an idempotent orchestration is executed to ensure the cluster is in the desired state. As part of the orchestration, the version of the operator
will be reviewed. If the version has changed, the operator will update each of the daemons in a predictable order such as: mon, mgr, osd, rgw, mds. If the Rook upgrade requires any special steps, they will be handled as each version upgrade requires.

When the cluster CRD is updated with a new Ceph version, the same idempotent orchestration is executed to evaluate desired state that needs to be applied to the cluster.
Over time as the operator becomes smarter and more versions are supported, the custom upgrade steps will be implemented as needed.

Daemons will only be restarted when necessary for the upgrade. The Rook upgrade sometimes will not require a restart of the daemons,
depending on if the pod spec changed. The Ceph upgrade will always require a restart of the daemons. In either case, a restart will be done in an orderly, rolling manner
with one pod at a time along with health checks as the upgrade proceeds. The upgrade will be paused if the cluster becomes unhealthy.

See the [Upgrade design doc](rook-upgrade.md) for more details on the general upgrade approach.

#### Admin control of upgrades

To allow more control over the upgrade, we define `upgradePolicy` settings. They will allow the admin to:
- Upgrade one type of daemon at a time and confirm they are healthy before continuing with the upgrade
- Allow for testing of future versions that are not officially supported

The settings in the CRD to accommodate the design include:
- `upgradePolicy.cephVersion`: The version of the image to start applying to the daemons specified in the `components` list.
  - `allowUnsupported`: If `false`, the operator would refuse to upgrade the Ceph version if it doesn't support or recognize that version. This would allow testing of upgrade to unreleased versions. The default is `false`.
- `upgradePolicy.components`: A list of daemons or other components that should be upgraded to the version `newCephVersion`. The daemons include `mon`, `osd`, `mgr`, `rgw`, and `mds`. The ordering of the list will be ignored as Rook will only support ordering as it determines necessary for a version. If there are special upgrade actions in the future, they could be named and added to this list.

For example, with the settings below the operator would only upgrade the mons to mimic, while other daemons would remain on luminous. When the admin is ready, he would add more daemons to the list.

```yaml
spec:
  cephVersion:
    image: ceph/ceph:v12.2.9-20181026
    allowUnsupported: false
  upgradePolicy:
    cephVersion:
      image: ceph/ceph:v13.2.2-20181023
      allowUnsupported: false
    components:
    - mon
```

When the admin is completed with the upgrade or he is ready to allow Rook to complete the full upgrade for all daemons, he would set `cephVersion.image: ceph/ceph:v13.2.2`, and the operator would ignore the `upgradePolicy` since the `cephVersion` and `upgradePolicy.cephVersion` match.

If the admin wants to pause or otherwise control the upgrade closely, there are a couple of natural back doors:
- Deleting the operator pod will effectively pause the upgrade. Starting the operator pod up again would resume the upgrade.
- If the admin wants to manually upgrade the daemons, he could stop the operator pod, then set the container image on each of the Deployments (pods) he wants to update. The difficulty with this approach is if there are any changes to the pod specs that are made between versions of the daemons. The admin could update the pod specs manually, but it would be error prone.

#### Developer controls

If a developer wants to test the upgrade from mimic to nautilus, he would first create the cluster based on mimic. Then he would update the crd with the "unrecognized version" attribute in the CRD to specify nautilus such as:
```yaml
spec:
  cephVersion:
    image: ceph/ceph:v14.1.1
    allowUnsupported: true
```

Until Nautilus builds are released, the latest Nautilus build can be tested by using the image `ceph/daemon-base:latest-master`.

### Default Version

For backward compatibility, if the `cephVersion` property is not set, the operator will need to internally set a default version of Ceph.
The operator will assume the desired Ceph version is Luminous 12.2.7, which was shipped with Rook v0.8.
This default will allow the Rook upgrade from v0.8 to v0.9 to only impact the Rook version and hold the Ceph version at Luminous.
After the Rook upgrade to v0.9, the user can choose to set the `cephVersion` property to some newer version of Ceph such as mimic.
