<p>Packages:</p>
<ul>
<li>
<a href="#ceph.rook.io%2fv1">ceph.rook.io/v1</a>
</li>
</ul>
<h2 id="ceph.rook.io/v1">ceph.rook.io/v1</h2>
<div>
<p>Package v1 is the v1 version of the API.</p>
</div>
Resource Types:
<ul><li>
<a href="#ceph.rook.io/v1.CephBlockPool">CephBlockPool</a>
</li><li>
<a href="#ceph.rook.io/v1.CephBucketNotification">CephBucketNotification</a>
</li><li>
<a href="#ceph.rook.io/v1.CephBucketTopic">CephBucketTopic</a>
</li><li>
<a href="#ceph.rook.io/v1.CephCOSIDriver">CephCOSIDriver</a>
</li><li>
<a href="#ceph.rook.io/v1.CephClient">CephClient</a>
</li><li>
<a href="#ceph.rook.io/v1.CephCluster">CephCluster</a>
</li><li>
<a href="#ceph.rook.io/v1.CephFilesystem">CephFilesystem</a>
</li><li>
<a href="#ceph.rook.io/v1.CephFilesystemMirror">CephFilesystemMirror</a>
</li><li>
<a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroup">CephFilesystemSubVolumeGroup</a>
</li><li>
<a href="#ceph.rook.io/v1.CephNFS">CephNFS</a>
</li><li>
<a href="#ceph.rook.io/v1.CephObjectRealm">CephObjectRealm</a>
</li><li>
<a href="#ceph.rook.io/v1.CephObjectStore">CephObjectStore</a>
</li><li>
<a href="#ceph.rook.io/v1.CephObjectStoreUser">CephObjectStoreUser</a>
</li><li>
<a href="#ceph.rook.io/v1.CephObjectZone">CephObjectZone</a>
</li><li>
<a href="#ceph.rook.io/v1.CephObjectZoneGroup">CephObjectZoneGroup</a>
</li><li>
<a href="#ceph.rook.io/v1.CephRBDMirror">CephRBDMirror</a>
</li></ul>
<h3 id="ceph.rook.io/v1.CephBlockPool">CephBlockPool
</h3>
<div>
<p>CephBlockPool represents a Ceph Storage Pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephBlockPool</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.NamedBlockPoolSpec">
NamedBlockPoolSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The desired name of the pool if different from the CephBlockPool CR name.</p>
</td>
</tr>
<tr>
<td>
<code>PoolSpec</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PoolSpec</code> are embedded into this type.)
</p>
<p>The core pool configuration</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephBlockPoolStatus">
CephBlockPoolStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephBucketNotification">CephBucketNotification
</h3>
<div>
<p>CephBucketNotification represents a Bucket Notifications</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephBucketNotification</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.BucketNotificationSpec">
BucketNotificationSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>topic</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the topic associated with this notification</p>
</td>
</tr>
<tr>
<td>
<code>events</code><br/>
<em>
<a href="#ceph.rook.io/v1.BucketNotificationEvent">
[]BucketNotificationEvent
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of events that should trigger the notification</p>
</td>
</tr>
<tr>
<td>
<code>filter</code><br/>
<em>
<a href="#ceph.rook.io/v1.NotificationFilterSpec">
NotificationFilterSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Spec of notification filter</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephBucketTopic">CephBucketTopic
</h3>
<div>
<p>CephBucketTopic represents a Ceph Object Topic for Bucket Notifications</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephBucketTopic</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.BucketTopicSpec">
BucketTopicSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>objectStoreName</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the object store on which to define the topic</p>
</td>
</tr>
<tr>
<td>
<code>objectStoreNamespace</code><br/>
<em>
string
</em>
</td>
<td>
<p>The namespace of the object store on which to define the topic</p>
</td>
</tr>
<tr>
<td>
<code>opaqueData</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Data which is sent in each event</p>
</td>
</tr>
<tr>
<td>
<code>persistent</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indication whether notifications to this endpoint are persistent or not</p>
</td>
</tr>
<tr>
<td>
<code>endpoint</code><br/>
<em>
<a href="#ceph.rook.io/v1.TopicEndpointSpec">
TopicEndpointSpec
</a>
</em>
</td>
<td>
<p>Contains the endpoint spec of the topic</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.BucketTopicStatus">
BucketTopicStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephCOSIDriver">CephCOSIDriver
</h3>
<div>
<p>CephCOSIDriver represents the CRD for the Ceph COSI Driver Deployment</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephCOSIDriver</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephCOSIDriverSpec">
CephCOSIDriverSpec
</a>
</em>
</td>
<td>
<p>Spec represents the specification of a Ceph COSI Driver</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Image is the container image to run the Ceph COSI driver</p>
</td>
</tr>
<tr>
<td>
<code>objectProvisionerImage</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObjectProvisionerImage is the container image to run the COSI driver sidecar</p>
</td>
</tr>
<tr>
<td>
<code>deploymentStrategy</code><br/>
<em>
<a href="#ceph.rook.io/v1.COSIDeploymentStrategy">
COSIDeploymentStrategy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DeploymentStrategy is the strategy to use to deploy the COSI driver.</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Placement is the placement strategy to use for the COSI driver</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources is the resource requirements for the COSI driver</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephClient">CephClient
</h3>
<div>
<p>CephClient represents a Ceph Client</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephClient</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClientSpec">
ClientSpec
</a>
</em>
</td>
<td>
<p>Spec represents the specification of a Ceph Client</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>caps</code><br/>
<em>
map[string]string
</em>
</td>
<td>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephClientStatus">
CephClientStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Status represents the status of a Ceph Client</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephCluster">CephCluster
</h3>
<div>
<p>CephCluster is a Ceph storage cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephCluster</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterSpec">
ClusterSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>cephVersion</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephVersionSpec">
CephVersionSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The version information that instructs Rook to orchestrate a particular version of Ceph.</p>
</td>
</tr>
<tr>
<td>
<code>storage</code><br/>
<em>
<a href="#ceph.rook.io/v1.StorageScopeSpec">
StorageScopeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for available storage in the cluster and how it should be used</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.AnnotationsSpec">
AnnotationsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.LabelsSpec">
LabelsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.PlacementSpec">
PlacementSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).</p>
</td>
</tr>
<tr>
<td>
<code>network</code><br/>
<em>
<a href="#ceph.rook.io/v1.NetworkSpec">
NetworkSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Network related configuration</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="#ceph.rook.io/v1.ResourceSpec">
ResourceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources set resource requests and limits</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassNames</code><br/>
<em>
<a href="#ceph.rook.io/v1.PriorityClassNamesSpec">
PriorityClassNamesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassNames sets priority classes on components</p>
</td>
</tr>
<tr>
<td>
<code>dataDirHostPath</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The path on the host where config and data can be persisted</p>
</td>
</tr>
<tr>
<td>
<code>skipUpgradeChecks</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>SkipUpgradeChecks defines if an upgrade should be forced even if one of the check fails</p>
</td>
</tr>
<tr>
<td>
<code>continueUpgradeAfterChecksEvenIfNotHealthy</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>ContinueUpgradeAfterChecksEvenIfNotHealthy defines if an upgrade should continue even if PGs are not clean</p>
</td>
</tr>
<tr>
<td>
<code>waitTimeoutForHealthyOSDInMinutes</code><br/>
<em>
time.Duration
</em>
</td>
<td>
<em>(Optional)</em>
<p>WaitTimeoutForHealthyOSDInMinutes defines the time the operator would wait before an OSD can be stopped for upgrade or restart.
If the timeout exceeds and OSD is not ok to stop, then the operator would skip upgrade for the current OSD and proceed with the next one
if <code>continueUpgradeAfterChecksEvenIfNotHealthy</code> is <code>false</code>. If <code>continueUpgradeAfterChecksEvenIfNotHealthy</code> is <code>true</code>, then operator would
continue with the upgrade of an OSD even if its not ok to stop after the timeout. This timeout won&rsquo;t be applied if <code>skipUpgradeChecks</code> is <code>true</code>.
The default wait timeout is 10 minutes.</p>
</td>
</tr>
<tr>
<td>
<code>upgradeOSDRequiresHealthyPGs</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>UpgradeOSDRequiresHealthyPGs defines if OSD upgrade requires PGs are clean. If set to <code>true</code> OSD upgrade process won&rsquo;t start until PGs are healthy.
This configuration will be ignored if <code>skipUpgradeChecks</code> is <code>true</code>.
Default is false.</p>
</td>
</tr>
<tr>
<td>
<code>disruptionManagement</code><br/>
<em>
<a href="#ceph.rook.io/v1.DisruptionManagementSpec">
DisruptionManagementSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for configuring disruption management.</p>
</td>
</tr>
<tr>
<td>
<code>mon</code><br/>
<em>
<a href="#ceph.rook.io/v1.MonSpec">
MonSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for mon related options</p>
</td>
</tr>
<tr>
<td>
<code>crashCollector</code><br/>
<em>
<a href="#ceph.rook.io/v1.CrashCollectorSpec">
CrashCollectorSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for the crash controller</p>
</td>
</tr>
<tr>
<td>
<code>dashboard</code><br/>
<em>
<a href="#ceph.rook.io/v1.DashboardSpec">
DashboardSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Dashboard settings</p>
</td>
</tr>
<tr>
<td>
<code>monitoring</code><br/>
<em>
<a href="#ceph.rook.io/v1.MonitoringSpec">
MonitoringSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Prometheus based Monitoring settings</p>
</td>
</tr>
<tr>
<td>
<code>external</code><br/>
<em>
<a href="#ceph.rook.io/v1.ExternalSpec">
ExternalSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether the Ceph Cluster is running external to this Kubernetes cluster
mon, mgr, osd, mds, and discover daemons will not be created for external clusters.</p>
</td>
</tr>
<tr>
<td>
<code>mgr</code><br/>
<em>
<a href="#ceph.rook.io/v1.MgrSpec">
MgrSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for mgr related options</p>
</td>
</tr>
<tr>
<td>
<code>removeOSDsIfOutAndSafeToRemove</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Remove the OSD that is out and safe to remove only if this option is true</p>
</td>
</tr>
<tr>
<td>
<code>cleanupPolicy</code><br/>
<em>
<a href="#ceph.rook.io/v1.CleanupPolicySpec">
CleanupPolicySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicates user intent when deleting a cluster; blocks orchestration and should not be set if cluster
deletion is not imminent.</p>
</td>
</tr>
<tr>
<td>
<code>healthCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephClusterHealthCheckSpec">
CephClusterHealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Internal daemon healthchecks and liveness probe</p>
</td>
</tr>
<tr>
<td>
<code>security</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterSecuritySpec">
ClusterSecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Security represents security settings</p>
</td>
</tr>
<tr>
<td>
<code>logCollector</code><br/>
<em>
<a href="#ceph.rook.io/v1.LogCollectorSpec">
LogCollectorSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Logging represents loggings settings</p>
</td>
</tr>
<tr>
<td>
<code>csi</code><br/>
<em>
<a href="#ceph.rook.io/v1.CSIDriverSpec">
CSIDriverSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>CSI Driver Options applied per cluster.</p>
</td>
</tr>
<tr>
<td>
<code>cephConfig</code><br/>
<em>
map[string]map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Ceph Config options</p>
</td>
</tr>
<tr>
<td>
<code>cephConfigFromSecret</code><br/>
<em>
map[string]map[string]k8s.io/api/core/v1.SecretKeySelector
</em>
</td>
<td>
<em>(Optional)</em>
<p>CephConfigFromSecret works exactly like CephConfig but takes config value from Secret Key reference.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterStatus">
ClusterStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystem">CephFilesystem
</h3>
<div>
<p>CephFilesystem represents a Ceph Filesystem</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephFilesystem</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemSpec">
FilesystemSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>metadataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.NamedPoolSpec">
NamedPoolSpec
</a>
</em>
</td>
<td>
<p>The metadata pool settings</p>
</td>
</tr>
<tr>
<td>
<code>dataPools</code><br/>
<em>
<a href="#ceph.rook.io/v1.NamedPoolSpec">
[]NamedPoolSpec
</a>
</em>
</td>
<td>
<p>The data pool settings, with optional predefined pool name.</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolNames</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pool names as specified</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolsOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pools on filesystem deletion</p>
</td>
</tr>
<tr>
<td>
<code>preserveFilesystemOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve the fs in the cluster on CephFilesystem CR deletion. Setting this to true automatically implies PreservePoolsOnDelete is true.</p>
</td>
</tr>
<tr>
<td>
<code>metadataServer</code><br/>
<em>
<a href="#ceph.rook.io/v1.MetadataServerSpec">
MetadataServerSpec
</a>
</em>
</td>
<td>
<p>The mds pod info</p>
</td>
</tr>
<tr>
<td>
<code>mirroring</code><br/>
<em>
<a href="#ceph.rook.io/v1.FSMirroringSpec">
FSMirroringSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The mirroring settings</p>
</td>
</tr>
<tr>
<td>
<code>statusCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirrorHealthCheckSpec">
MirrorHealthCheckSpec
</a>
</em>
</td>
<td>
<p>The mirroring statusCheck</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephFilesystemStatus">
CephFilesystemStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystemMirror">CephFilesystemMirror
</h3>
<div>
<p>CephFilesystemMirror is the Ceph Filesystem Mirror object definition</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephFilesystemMirror</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemMirroringSpec">
FilesystemMirroringSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the rgw pods (default is to place on any available node)</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The resource requirements for the cephfs-mirror pods</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets priority class on the cephfs-mirror pods</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystemSubVolumeGroup">CephFilesystemSubVolumeGroup
</h3>
<div>
<p>CephFilesystemSubVolumeGroup represents a Ceph Filesystem SubVolumeGroup</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephFilesystemSubVolumeGroup</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroupSpec">
CephFilesystemSubVolumeGroupSpec
</a>
</em>
</td>
<td>
<p>Spec represents the specification of a Ceph Filesystem SubVolumeGroup</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the subvolume group. If not set, the default is the name of the subvolumeGroup CR.</p>
</td>
</tr>
<tr>
<td>
<code>filesystemName</code><br/>
<em>
string
</em>
</td>
<td>
<p>FilesystemName is the name of Ceph Filesystem SubVolumeGroup volume name. Typically it&rsquo;s the name of
the CephFilesystem CR. If not coming from the CephFilesystem CR, it can be retrieved from the
list of Ceph Filesystem volumes with <code>ceph fs volume ls</code>. To learn more about Ceph Filesystem
abstractions see <a href="https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-volumes-and-subvolumes">https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-volumes-and-subvolumes</a></p>
</td>
</tr>
<tr>
<td>
<code>pinning</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroupSpecPinning">
CephFilesystemSubVolumeGroupSpecPinning
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Pinning configuration of CephFilesystemSubVolumeGroup,
reference <a href="https://docs.ceph.com/en/latest/cephfs/fs-volumes/#pinning-subvolumes-and-subvolume-groups">https://docs.ceph.com/en/latest/cephfs/fs-volumes/#pinning-subvolumes-and-subvolume-groups</a>
only one out of (export, distributed, random) can be set at a time</p>
</td>
</tr>
<tr>
<td>
<code>quota</code><br/>
<em>
k8s.io/apimachinery/pkg/api/resource.Quantity
</em>
</td>
<td>
<em>(Optional)</em>
<p>Quota size of the Ceph Filesystem subvolume group.</p>
</td>
</tr>
<tr>
<td>
<code>dataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool name for the Ceph Filesystem subvolume group layout, if the default CephFS pool is not desired.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroupStatus">
CephFilesystemSubVolumeGroupStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Status represents the status of a CephFilesystem SubvolumeGroup</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephNFS">CephNFS
</h3>
<div>
<p>CephNFS represents a Ceph NFS</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephNFS</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.NFSGaneshaSpec">
NFSGaneshaSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>rados</code><br/>
<em>
<a href="#ceph.rook.io/v1.GaneshaRADOSSpec">
GaneshaRADOSSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RADOS is the Ganesha RADOS specification</p>
</td>
</tr>
<tr>
<td>
<code>server</code><br/>
<em>
<a href="#ceph.rook.io/v1.GaneshaServerSpec">
GaneshaServerSpec
</a>
</em>
</td>
<td>
<p>Server is the Ganesha Server specification</p>
</td>
</tr>
<tr>
<td>
<code>security</code><br/>
<em>
<a href="#ceph.rook.io/v1.NFSSecuritySpec">
NFSSecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Security allows specifying security configurations for the NFS cluster</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephObjectRealm">CephObjectRealm
</h3>
<div>
<p>CephObjectRealm represents a Ceph Object Store Gateway Realm</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephObjectRealm</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectRealmSpec">
ObjectRealmSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<br/>
<br/>
<table>
<tr>
<td>
<code>pull</code><br/>
<em>
<a href="#ceph.rook.io/v1.PullSpec">
PullSpec
</a>
</em>
</td>
<td>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephObjectStore">CephObjectStore
</h3>
<div>
<p>CephObjectStore represents a Ceph Object Store Gateway</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephObjectStore</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreSpec">
ObjectStoreSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>metadataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The metadata pool settings</p>
</td>
</tr>
<tr>
<td>
<code>dataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool settings</p>
</td>
</tr>
<tr>
<td>
<code>sharedPools</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectSharedPoolsSpec">
ObjectSharedPoolsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The pool information when configuring RADOS namespaces in existing pools.</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolsOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pools on object store deletion</p>
</td>
</tr>
<tr>
<td>
<code>gateway</code><br/>
<em>
<a href="#ceph.rook.io/v1.GatewaySpec">
GatewaySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The rgw pod info</p>
</td>
</tr>
<tr>
<td>
<code>protocols</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProtocolSpec">
ProtocolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol specification</p>
</td>
</tr>
<tr>
<td>
<code>auth</code><br/>
<em>
<a href="#ceph.rook.io/v1.AuthSpec">
AuthSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The authentication configuration</p>
</td>
</tr>
<tr>
<td>
<code>zone</code><br/>
<em>
<a href="#ceph.rook.io/v1.ZoneSpec">
ZoneSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The multisite info</p>
</td>
</tr>
<tr>
<td>
<code>healthCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectHealthCheckSpec">
ObjectHealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The RGW health probes</p>
</td>
</tr>
<tr>
<td>
<code>security</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreSecuritySpec">
ObjectStoreSecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Security represents security settings</p>
</td>
</tr>
<tr>
<td>
<code>allowUsersInNamespaces</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The list of allowed namespaces in addition to the object store namespace
where ceph object store users may be created. Specify &ldquo;*&rdquo; to allow all
namespaces, otherwise list individual namespaces that are to be allowed.
This is useful for applications that need object store credentials
to be created in their own namespace, where neither OBCs nor COSI
is being used to create buckets. The default is empty.</p>
</td>
</tr>
<tr>
<td>
<code>hosting</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreHostingSpec">
ObjectStoreHostingSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Hosting settings for the object store.
A common use case for hosting configuration is to inform Rook of endpoints that support DNS
wildcards, which in turn allows virtual host-style bucket addressing.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreStatus">
ObjectStoreStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephObjectStoreUser">CephObjectStoreUser
</h3>
<div>
<p>CephObjectStoreUser represents a Ceph Object Store Gateway User</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephObjectStoreUser</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreUserSpec">
ObjectStoreUserSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>store</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The store the user will be created in</p>
</td>
</tr>
<tr>
<td>
<code>displayName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The display name for the ceph users</p>
</td>
</tr>
<tr>
<td>
<code>capabilities</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectUserCapSpec">
ObjectUserCapSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>quotas</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectUserQuotaSpec">
ObjectUserQuotaSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>keys</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectUserKey">
[]ObjectUserKey
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Allows specifying credentials for the user. If not provided, the operator
will generate them.</p>
</td>
</tr>
<tr>
<td>
<code>clusterNamespace</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The namespace where the parent CephCluster and CephObjectStore are found</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreUserStatus">
ObjectStoreUserStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephObjectZone">CephObjectZone
</h3>
<div>
<p>CephObjectZone represents a Ceph Object Store Gateway Zone</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephObjectZone</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectZoneSpec">
ObjectZoneSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>zoneGroup</code><br/>
<em>
string
</em>
</td>
<td>
<p>The display name for the ceph users</p>
</td>
</tr>
<tr>
<td>
<code>metadataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The metadata pool settings</p>
</td>
</tr>
<tr>
<td>
<code>dataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool settings</p>
</td>
</tr>
<tr>
<td>
<code>sharedPools</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectSharedPoolsSpec">
ObjectSharedPoolsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The pool information when configuring RADOS namespaces in existing pools.</p>
</td>
</tr>
<tr>
<td>
<code>customEndpoints</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If this zone cannot be accessed from other peer Ceph clusters via the ClusterIP Service
endpoint created by Rook, you must set this to the externally reachable endpoint(s). You may
include the port in the definition. For example: &ldquo;<a href="https://my-object-store.my-domain.net:443&quot;">https://my-object-store.my-domain.net:443&rdquo;</a>.
In many cases, you should set this to the endpoint of the ingress resource that makes the
CephObjectStore associated with this CephObjectStoreZone reachable to peer clusters.
The list can have one or more endpoints pointing to different RGW servers in the zone.</p>
<p>If a CephObjectStore endpoint is omitted from this list, that object store&rsquo;s gateways will
not receive multisite replication data
(see CephObjectStore.spec.gateway.disableMultisiteSyncTraffic).</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolsOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pools on object zone deletion</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephObjectZoneGroup">CephObjectZoneGroup
</h3>
<div>
<p>CephObjectZoneGroup represents a Ceph Object Store Gateway Zone Group</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephObjectZoneGroup</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectZoneGroupSpec">
ObjectZoneGroupSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>realm</code><br/>
<em>
string
</em>
</td>
<td>
<p>The display name for the ceph users</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephRBDMirror">CephRBDMirror
</h3>
<div>
<p>CephRBDMirror represents a Ceph RBD Mirror</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
ceph.rook.io/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>CephRBDMirror</code></td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.RBDMirroringSpec">
RBDMirroringSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>count</code><br/>
<em>
int
</em>
</td>
<td>
<p>Count represents the number of rbd mirror instance to run</p>
</td>
</tr>
<tr>
<td>
<code>peers</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringPeerSpec">
MirroringPeerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Peers represents the peers spec</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the rgw pods (default is to place on any available node)</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The resource requirements for the rbd mirror pods</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets priority class on the rbd mirror pods</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.Status">
Status
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.AMQPEndpointSpec">AMQPEndpointSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.TopicEndpointSpec">TopicEndpointSpec</a>)
</p>
<div>
<p>AMQPEndpointSpec represent the spec of an AMQP endpoint of a Bucket Topic</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uri</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URI of the AMQP endpoint to push notification to</p>
</td>
</tr>
<tr>
<td>
<code>exchange</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name of the exchange that is used to route messages based on topics</p>
</td>
</tr>
<tr>
<td>
<code>disableVerifySSL</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicate whether the server certificate is validated by the client or not</p>
</td>
</tr>
<tr>
<td>
<code>ackLevel</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The ack level required for this topic (none/broker/routeable)</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.AdditionalVolumeMount">AdditionalVolumeMount
</h3>
<div>
<p>AdditionalVolumeMount represents the source from where additional files in pod containers
should come from and what subdirectory they are made available in.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>subPath</code><br/>
<em>
string
</em>
</td>
<td>
<p>SubPath defines the sub-path (subdirectory) of the directory root where the volumeSource will
be mounted. All files/keys in the volume source&rsquo;s volume will be mounted to the subdirectory.
This is not the same as the Kubernetes <code>subPath</code> volume mount option.
Each subPath definition must be unique and must not contain &lsquo;:&rsquo;.</p>
</td>
</tr>
<tr>
<td>
<code>volumeSource</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConfigFileVolumeSource">
ConfigFileVolumeSource
</a>
</em>
</td>
<td>
<p>VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for the
additional file(s) like what is normally used to configure Volumes for a Pod. Fore example, a
ConfigMap, Secret, or HostPath. Each VolumeSource adds one or more additional files to the
container <code>&lt;directory-root&gt;/&lt;subPath&gt;</code> directory.
Be aware that some files may need to have a specific file mode like 0600 due to application
requirements. For example, CA or TLS certificates.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.AdditionalVolumeMounts">AdditionalVolumeMounts
(<code>[]github.com/rook/rook/pkg/apis/ceph.rook.io/v1.AdditionalVolumeMount</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>, <a href="#ceph.rook.io/v1.SSSDSidecar">SSSDSidecar</a>)
</p>
<div>
</div>
<h3 id="ceph.rook.io/v1.AddressRangesSpec">AddressRangesSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NetworkSpec">NetworkSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>public</code><br/>
<em>
<a href="#ceph.rook.io/v1.CIDRList">
CIDRList
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Public defines a list of CIDRs to use for Ceph public network communication.</p>
</td>
</tr>
<tr>
<td>
<code>cluster</code><br/>
<em>
<a href="#ceph.rook.io/v1.CIDRList">
CIDRList
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Cluster defines a list of CIDRs to use for Ceph cluster network communication.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Annotations">Annotations
(<code>map[string]string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemMirroringSpec">FilesystemMirroringSpec</a>, <a href="#ceph.rook.io/v1.GaneshaServerSpec">GaneshaServerSpec</a>, <a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>, <a href="#ceph.rook.io/v1.MetadataServerSpec">MetadataServerSpec</a>, <a href="#ceph.rook.io/v1.RBDMirroringSpec">RBDMirroringSpec</a>, <a href="#ceph.rook.io/v1.RGWServiceSpec">RGWServiceSpec</a>)
</p>
<div>
<p>Annotations are annotations</p>
</div>
<h3 id="ceph.rook.io/v1.AnnotationsSpec">AnnotationsSpec
(<code>map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.KeyType]github.com/rook/rook/pkg/apis/ceph.rook.io/v1.Annotations</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>AnnotationsSpec is the main spec annotation for all daemons</p>
</div>
<h3 id="ceph.rook.io/v1.AuthSpec">AuthSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>AuthSpec represents the authentication protocol configuration of a Ceph Object Store Gateway</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>keystone</code><br/>
<em>
<a href="#ceph.rook.io/v1.KeystoneSpec">
KeystoneSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The spec for Keystone</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.BucketNotificationEvent">BucketNotificationEvent
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.BucketNotificationSpec">BucketNotificationSpec</a>)
</p>
<div>
<p>BucketNotificationSpec represent the event type of the bucket notification</p>
</div>
<h3 id="ceph.rook.io/v1.BucketNotificationSpec">BucketNotificationSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBucketNotification">CephBucketNotification</a>)
</p>
<div>
<p>BucketNotificationSpec represent the spec of a Bucket Notification</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>topic</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the topic associated with this notification</p>
</td>
</tr>
<tr>
<td>
<code>events</code><br/>
<em>
<a href="#ceph.rook.io/v1.BucketNotificationEvent">
[]BucketNotificationEvent
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of events that should trigger the notification</p>
</td>
</tr>
<tr>
<td>
<code>filter</code><br/>
<em>
<a href="#ceph.rook.io/v1.NotificationFilterSpec">
NotificationFilterSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Spec of notification filter</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.BucketTopicSpec">BucketTopicSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBucketTopic">CephBucketTopic</a>)
</p>
<div>
<p>BucketTopicSpec represent the spec of a Bucket Topic</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>objectStoreName</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the object store on which to define the topic</p>
</td>
</tr>
<tr>
<td>
<code>objectStoreNamespace</code><br/>
<em>
string
</em>
</td>
<td>
<p>The namespace of the object store on which to define the topic</p>
</td>
</tr>
<tr>
<td>
<code>opaqueData</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Data which is sent in each event</p>
</td>
</tr>
<tr>
<td>
<code>persistent</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indication whether notifications to this endpoint are persistent or not</p>
</td>
</tr>
<tr>
<td>
<code>endpoint</code><br/>
<em>
<a href="#ceph.rook.io/v1.TopicEndpointSpec">
TopicEndpointSpec
</a>
</em>
</td>
<td>
<p>Contains the endpoint spec of the topic</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.BucketTopicStatus">BucketTopicStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBucketTopic">CephBucketTopic</a>)
</p>
<div>
<p>BucketTopicStatus represents the Status of a CephBucketTopic</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>ARN</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The ARN of the topic generated by the RGW</p>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
<tr>
<td>
<code>secrets</code><br/>
<em>
<a href="#ceph.rook.io/v1.SecretReference">
[]SecretReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CIDR">CIDR
(<code>string</code> alias)</h3>
<div>
<p>An IPv4 or IPv6 network CIDR.</p>
<p>This naive kubebuilder regex provides immediate feedback for some typos and for a common problem
case where the range spec is forgotten (e.g., /24). Rook does in-depth validation in code.</p>
</div>
<h3 id="ceph.rook.io/v1.COSIDeploymentStrategy">COSIDeploymentStrategy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephCOSIDriverSpec">CephCOSIDriverSpec</a>)
</p>
<div>
<p>COSIDeploymentStrategy represents the strategy to use to deploy the Ceph COSI driver</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Always&#34;</p></td>
<td><p>Always means the Ceph COSI driver will be deployed even if the object store is not present</p>
</td>
</tr><tr><td><p>&#34;Auto&#34;</p></td>
<td><p>Auto means the Ceph COSI driver will be deployed automatically if object store is present</p>
</td>
</tr><tr><td><p>&#34;Never&#34;</p></td>
<td><p>Never means the Ceph COSI driver will never deployed</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.CSICephFSSpec">CSICephFSSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CSIDriverSpec">CSIDriverSpec</a>)
</p>
<div>
<p>CSICephFSSpec defines the settings for CephFS CSI driver.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kernelMountOptions</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>KernelMountOptions defines the mount options for kernel mounter.</p>
</td>
</tr>
<tr>
<td>
<code>fuseMountOptions</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>FuseMountOptions defines the mount options for ceph fuse mounter.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CSIDriverSpec">CSIDriverSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>CSIDriverSpec defines CSI Driver settings applied per cluster.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>readAffinity</code><br/>
<em>
<a href="#ceph.rook.io/v1.ReadAffinitySpec">
ReadAffinitySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReadAffinity defines the read affinity settings for CSI driver.</p>
</td>
</tr>
<tr>
<td>
<code>cephfs</code><br/>
<em>
<a href="#ceph.rook.io/v1.CSICephFSSpec">
CSICephFSSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>CephFS defines CSI Driver settings for CephFS driver.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Capacity">Capacity
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephStatus">CephStatus</a>)
</p>
<div>
<p>Capacity is the capacity information of a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>bytesTotal</code><br/>
<em>
uint64
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bytesUsed</code><br/>
<em>
uint64
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>bytesAvailable</code><br/>
<em>
uint64
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lastUpdated</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephBlockPoolRadosNamespace">CephBlockPoolRadosNamespace
</h3>
<div>
<p>CephBlockPoolRadosNamespace represents a Ceph BlockPool Rados Namespace</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceSpec">
CephBlockPoolRadosNamespaceSpec
</a>
</em>
</td>
<td>
<p>Spec represents the specification of a Ceph BlockPool Rados Namespace</p>
<br/>
<br/>
<table>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the CephBlockPoolRadosNamespaceSpec namespace. If not set, the default is the name of the CR.</p>
</td>
</tr>
<tr>
<td>
<code>blockPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<p>BlockPoolName is the name of Ceph BlockPool. Typically it&rsquo;s the name of
the CephBlockPool CR.</p>
</td>
</tr>
<tr>
<td>
<code>mirroring</code><br/>
<em>
<a href="#ceph.rook.io/v1.RadosNamespaceMirroring">
RadosNamespaceMirroring
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mirroring configuration of CephBlockPoolRadosNamespace</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">
CephBlockPoolRadosNamespaceStatus
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Status represents the status of a CephBlockPool Rados Namespace</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephBlockPoolRadosNamespaceSpec">CephBlockPoolRadosNamespaceSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespace">CephBlockPoolRadosNamespace</a>)
</p>
<div>
<p>CephBlockPoolRadosNamespaceSpec represents the specification of a CephBlockPool Rados Namespace</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the CephBlockPoolRadosNamespaceSpec namespace. If not set, the default is the name of the CR.</p>
</td>
</tr>
<tr>
<td>
<code>blockPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<p>BlockPoolName is the name of Ceph BlockPool. Typically it&rsquo;s the name of
the CephBlockPool CR.</p>
</td>
</tr>
<tr>
<td>
<code>mirroring</code><br/>
<em>
<a href="#ceph.rook.io/v1.RadosNamespaceMirroring">
RadosNamespaceMirroring
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mirroring configuration of CephBlockPoolRadosNamespace</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">CephBlockPoolRadosNamespaceStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespace">CephBlockPoolRadosNamespace</a>)
</p>
<div>
<p>CephBlockPoolRadosNamespaceStatus represents the Status of Ceph BlockPool
Rados Namespace</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>mirroringStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringStatusSpec">
MirroringStatusSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>mirroringInfo</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringInfoSpec">
MirroringInfoSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>snapshotScheduleStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotScheduleStatusSpec">
SnapshotScheduleStatusSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#ceph.rook.io/v1.Condition">
[]Condition
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephBlockPoolStatus">CephBlockPoolStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPool">CephBlockPool</a>)
</p>
<div>
<p>CephBlockPoolStatus represents the mirroring status of Ceph Storage Pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>mirroringStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringStatusSpec">
MirroringStatusSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>mirroringInfo</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringInfoSpec">
MirroringInfoSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>poolID</code><br/>
<em>
int
</em>
</td>
<td>
<p>optional</p>
</td>
</tr>
<tr>
<td>
<code>snapshotScheduleStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotScheduleStatusSpec">
SnapshotScheduleStatusSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#ceph.rook.io/v1.Condition">
[]Condition
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephCOSIDriverSpec">CephCOSIDriverSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephCOSIDriver">CephCOSIDriver</a>)
</p>
<div>
<p>CephCOSIDriverSpec represents the specification of a Ceph COSI Driver</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Image is the container image to run the Ceph COSI driver</p>
</td>
</tr>
<tr>
<td>
<code>objectProvisionerImage</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObjectProvisionerImage is the container image to run the COSI driver sidecar</p>
</td>
</tr>
<tr>
<td>
<code>deploymentStrategy</code><br/>
<em>
<a href="#ceph.rook.io/v1.COSIDeploymentStrategy">
COSIDeploymentStrategy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DeploymentStrategy is the strategy to use to deploy the COSI driver.</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Placement is the placement strategy to use for the COSI driver</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources is the resource requirements for the COSI driver</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephClientStatus">CephClientStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephClient">CephClient</a>)
</p>
<div>
<p>CephClientStatus represents the Status of Ceph Client</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephClusterHealthCheckSpec">CephClusterHealthCheckSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>CephClusterHealthCheckSpec represent the healthcheck for Ceph daemons</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>daemonHealth</code><br/>
<em>
<a href="#ceph.rook.io/v1.DaemonHealthSpec">
DaemonHealthSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DaemonHealth is the health check for a given daemon</p>
</td>
</tr>
<tr>
<td>
<code>livenessProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.*github.com/rook/rook/pkg/apis/ceph.rook.io/v1.ProbeSpec">
map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.KeyType]*github.com/rook/rook/pkg/apis/ceph.rook.io/v1.ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>LivenessProbe allows changing the livenessProbe configuration for a given daemon</p>
</td>
</tr>
<tr>
<td>
<code>startupProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.*github.com/rook/rook/pkg/apis/ceph.rook.io/v1.ProbeSpec">
map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.KeyType]*github.com/rook/rook/pkg/apis/ceph.rook.io/v1.ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>StartupProbe allows changing the startupProbe configuration for a given daemon</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephDaemonsVersions">CephDaemonsVersions
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephStatus">CephStatus</a>)
</p>
<div>
<p>CephDaemonsVersions show the current ceph version for different ceph daemons</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>mon</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mon shows Mon Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>mgr</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mgr shows Mgr Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>osd</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Osd shows Osd Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>rgw</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Rgw shows Rgw Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>mds</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mds shows Mds Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>rbd-mirror</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>RbdMirror shows RbdMirror Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>cephfs-mirror</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>CephFSMirror shows CephFSMirror Ceph version</p>
</td>
</tr>
<tr>
<td>
<code>overall</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Overall shows overall Ceph version</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephExporterSpec">CephExporterSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MonitoringSpec">MonitoringSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>perfCountersPrioLimit</code><br/>
<em>
int64
</em>
</td>
<td>
<p>Only performance counters greater than or equal to this option are fetched</p>
</td>
</tr>
<tr>
<td>
<code>statsPeriodSeconds</code><br/>
<em>
int64
</em>
</td>
<td>
<p>Time to wait before sending requests again to exporter server (seconds)</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystemStatus">CephFilesystemStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystem">CephFilesystem</a>)
</p>
<div>
<p>CephFilesystemStatus represents the status of a Ceph Filesystem</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>snapshotScheduleStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemSnapshotScheduleStatusSpec">
FilesystemSnapshotScheduleStatusSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Use only info and put mirroringStatus in it?</p>
</td>
</tr>
<tr>
<td>
<code>mirroringStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemMirroringInfoSpec">
FilesystemMirroringInfoSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>MirroringStatus is the filesystem mirroring status</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#ceph.rook.io/v1.Condition">
[]Condition
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystemSubVolumeGroupSpec">CephFilesystemSubVolumeGroupSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroup">CephFilesystemSubVolumeGroup</a>)
</p>
<div>
<p>CephFilesystemSubVolumeGroupSpec represents the specification of a Ceph Filesystem SubVolumeGroup</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the subvolume group. If not set, the default is the name of the subvolumeGroup CR.</p>
</td>
</tr>
<tr>
<td>
<code>filesystemName</code><br/>
<em>
string
</em>
</td>
<td>
<p>FilesystemName is the name of Ceph Filesystem SubVolumeGroup volume name. Typically it&rsquo;s the name of
the CephFilesystem CR. If not coming from the CephFilesystem CR, it can be retrieved from the
list of Ceph Filesystem volumes with <code>ceph fs volume ls</code>. To learn more about Ceph Filesystem
abstractions see <a href="https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-volumes-and-subvolumes">https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-volumes-and-subvolumes</a></p>
</td>
</tr>
<tr>
<td>
<code>pinning</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroupSpecPinning">
CephFilesystemSubVolumeGroupSpecPinning
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Pinning configuration of CephFilesystemSubVolumeGroup,
reference <a href="https://docs.ceph.com/en/latest/cephfs/fs-volumes/#pinning-subvolumes-and-subvolume-groups">https://docs.ceph.com/en/latest/cephfs/fs-volumes/#pinning-subvolumes-and-subvolume-groups</a>
only one out of (export, distributed, random) can be set at a time</p>
</td>
</tr>
<tr>
<td>
<code>quota</code><br/>
<em>
k8s.io/apimachinery/pkg/api/resource.Quantity
</em>
</td>
<td>
<em>(Optional)</em>
<p>Quota size of the Ceph Filesystem subvolume group.</p>
</td>
</tr>
<tr>
<td>
<code>dataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool name for the Ceph Filesystem subvolume group layout, if the default CephFS pool is not desired.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystemSubVolumeGroupSpecPinning">CephFilesystemSubVolumeGroupSpecPinning
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroupSpec">CephFilesystemSubVolumeGroupSpec</a>)
</p>
<div>
<p>CephFilesystemSubVolumeGroupSpecPinning represents the pinning configuration of SubVolumeGroup</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>export</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>distributed</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>random,</code><br/>
<em>
float64
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephFilesystemSubVolumeGroupStatus">CephFilesystemSubVolumeGroupStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroup">CephFilesystemSubVolumeGroup</a>)
</p>
<div>
<p>CephFilesystemSubVolumeGroupStatus represents the Status of Ceph Filesystem SubVolumeGroup</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephHealthMessage">CephHealthMessage
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephStatus">CephStatus</a>)
</p>
<div>
<p>CephHealthMessage represents the health message of a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>severity</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephNetworkType">CephNetworkType
(<code>string</code> alias)</h3>
<div>
<p>CephNetworkType should be &ldquo;public&rdquo; or &ldquo;cluster&rdquo;.
Allow any string so that over-specified legacy clusters do not break on CRD update.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;cluster&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;public&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.CephStatus">CephStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterStatus">ClusterStatus</a>)
</p>
<div>
<p>CephStatus is the details health of a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>health</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>details</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephHealthMessage">
map[string]github.com/rook/rook/pkg/apis/ceph.rook.io/v1.CephHealthMessage
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lastChecked</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lastChanged</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>previousHealth</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>capacity</code><br/>
<em>
<a href="#ceph.rook.io/v1.Capacity">
Capacity
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>versions</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephDaemonsVersions">
CephDaemonsVersions
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>fsid</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephStorage">CephStorage
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterStatus">ClusterStatus</a>)
</p>
<div>
<p>CephStorage represents flavors of Ceph Cluster Storage</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>deviceClasses</code><br/>
<em>
<a href="#ceph.rook.io/v1.DeviceClasses">
[]DeviceClasses
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>osd</code><br/>
<em>
<a href="#ceph.rook.io/v1.OSDStatus">
OSDStatus
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>deprecatedOSDs</code><br/>
<em>
map[string][]int
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephVersionSpec">CephVersionSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>CephVersionSpec represents the settings for the Ceph version that Rook is orchestrating.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Image is the container image used to launch the ceph daemons, such as quay.io/ceph/ceph:<tag>
The full list of images can be found at <a href="https://quay.io/repository/ceph/ceph?tab=tags">https://quay.io/repository/ceph/ceph?tab=tags</a></p>
</td>
</tr>
<tr>
<td>
<code>allowUnsupported</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to allow unsupported versions (do not set to true in production)</p>
</td>
</tr>
<tr>
<td>
<code>imagePullPolicy</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#pullpolicy-v1-core">
Kubernetes core/v1.PullPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImagePullPolicy describes a policy for if/when to pull a container image
One of Always, Never, IfNotPresent.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephxConfig">CephxConfig
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterCephxConfig">ClusterCephxConfig</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>keyRotationPolicy</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephxKeyRotationPolicy">
CephxKeyRotationPolicy
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeyRotationPolicy controls if and when CephX keys are rotated after initial creation.
One of Disabled, or KeyGeneration. Default Disabled.</p>
</td>
</tr>
<tr>
<td>
<code>keyGeneration</code><br/>
<em>
uint32
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeyGeneration specifies the desired CephX key generation. This is used when KeyRotationPolicy
is KeyGeneration and ignored for other policies. If this is set to greater than the current
key generation, relevant keys will be rotated, and the generation value will be updated to
this new value (generation values are not necessarily incremental, though that is the
intended use case). If this is set to less than or equal to the current key generation, keys
are not rotated.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CephxKeyRotationPolicy">CephxKeyRotationPolicy
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephxConfig">CephxConfig</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Disabled&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;KeyGeneration&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.CephxStatus">CephxStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.LocalCephxStatus">LocalCephxStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>keyGeneration</code><br/>
<em>
uint32
</em>
</td>
<td>
<p>KeyGeneration represents the CephX key generation for the last successful reconcile.
For all newly-created resources, this field is set to <code>1</code>.
When keys are rotated due to any rotation policy, the generation is incremented or updated to
the configured policy generation.
Generation <code>0</code> indicates that keys existed prior to the implementation of key tracking.</p>
</td>
</tr>
<tr>
<td>
<code>keyCephVersion</code><br/>
<em>
string
</em>
</td>
<td>
<p>KeyCephVersion reports the Ceph version that created the current generation&rsquo;s keys. This is
same string format as reported by <code>CephCluster.status.version.version</code> to allow them to be
compared. E.g., <code>20.2.0-0</code>.
For all newly-created resources, this field set to the version of Ceph that created the key.
The special value &ldquo;Uninitialized&rdquo; indicates that keys are being created for the first time.
An empty string indicates that the version is unknown, as expected in brownfield deployments.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CleanupConfirmationProperty">CleanupConfirmationProperty
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CleanupPolicySpec">CleanupPolicySpec</a>)
</p>
<div>
<p>CleanupConfirmationProperty represents the cleanup confirmation</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;yes-really-destroy-data&#34;</p></td>
<td><p>DeleteDataDirOnHostsConfirmation represents the validation to destroy dataDirHostPath</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.CleanupPolicySpec">CleanupPolicySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>CleanupPolicySpec represents a Ceph Cluster cleanup policy</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>confirmation</code><br/>
<em>
<a href="#ceph.rook.io/v1.CleanupConfirmationProperty">
CleanupConfirmationProperty
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Confirmation represents the cleanup confirmation</p>
</td>
</tr>
<tr>
<td>
<code>sanitizeDisks</code><br/>
<em>
<a href="#ceph.rook.io/v1.SanitizeDisksSpec">
SanitizeDisksSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SanitizeDisks represents way we sanitize disks</p>
</td>
</tr>
<tr>
<td>
<code>allowUninstallWithVolumes</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AllowUninstallWithVolumes defines whether we can proceed with the uninstall if they are RBD images still present</p>
</td>
</tr>
<tr>
<td>
<code>wipeDevicesFromOtherClusters</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>WipeDevicesFromOtherClusters wipes the OSD disks belonging to other clusters. This is useful in scenarios where ceph cluster
was reinstalled but OSD disk still contains the metadata from previous ceph cluster.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ClientSpec">ClientSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephClient">CephClient</a>)
</p>
<div>
<p>ClientSpec represents the specification of a Ceph Client</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>caps</code><br/>
<em>
map[string]string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ClusterCephxConfig">ClusterCephxConfig
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSecuritySpec">ClusterSecuritySpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>daemon</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephxConfig">
CephxConfig
</a>
</em>
</td>
<td>
<p>Daemon configures CephX key settings for local Ceph daemons managed by Rook and part of the
Ceph cluster. Daemon CephX keys can be rotated without affecting client connections.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ClusterSecuritySpec">ClusterSecuritySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>ClusterSecuritySpec is the CephCluster security spec to include various security items such as kms</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kms</code><br/>
<em>
<a href="#ceph.rook.io/v1.KeyManagementServiceSpec">
KeyManagementServiceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeyManagementService is the main Key Management option</p>
</td>
</tr>
<tr>
<td>
<code>keyRotation</code><br/>
<em>
<a href="#ceph.rook.io/v1.KeyRotationSpec">
KeyRotationSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeyRotation defines options for rotation of OSD disk encryption keys.</p>
</td>
</tr>
<tr>
<td>
<code>cephx</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterCephxConfig">
ClusterCephxConfig
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>CephX configures CephX key settings. More: <a href="https://docs.ceph.com/en/latest/dev/cephx/">https://docs.ceph.com/en/latest/dev/cephx/</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ClusterSpec">ClusterSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephCluster">CephCluster</a>)
</p>
<div>
<p>ClusterSpec represents the specification of Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>cephVersion</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephVersionSpec">
CephVersionSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The version information that instructs Rook to orchestrate a particular version of Ceph.</p>
</td>
</tr>
<tr>
<td>
<code>storage</code><br/>
<em>
<a href="#ceph.rook.io/v1.StorageScopeSpec">
StorageScopeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for available storage in the cluster and how it should be used</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.AnnotationsSpec">
AnnotationsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.LabelsSpec">
LabelsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.PlacementSpec">
PlacementSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).</p>
</td>
</tr>
<tr>
<td>
<code>network</code><br/>
<em>
<a href="#ceph.rook.io/v1.NetworkSpec">
NetworkSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Network related configuration</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="#ceph.rook.io/v1.ResourceSpec">
ResourceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources set resource requests and limits</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassNames</code><br/>
<em>
<a href="#ceph.rook.io/v1.PriorityClassNamesSpec">
PriorityClassNamesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassNames sets priority classes on components</p>
</td>
</tr>
<tr>
<td>
<code>dataDirHostPath</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The path on the host where config and data can be persisted</p>
</td>
</tr>
<tr>
<td>
<code>skipUpgradeChecks</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>SkipUpgradeChecks defines if an upgrade should be forced even if one of the check fails</p>
</td>
</tr>
<tr>
<td>
<code>continueUpgradeAfterChecksEvenIfNotHealthy</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>ContinueUpgradeAfterChecksEvenIfNotHealthy defines if an upgrade should continue even if PGs are not clean</p>
</td>
</tr>
<tr>
<td>
<code>waitTimeoutForHealthyOSDInMinutes</code><br/>
<em>
time.Duration
</em>
</td>
<td>
<em>(Optional)</em>
<p>WaitTimeoutForHealthyOSDInMinutes defines the time the operator would wait before an OSD can be stopped for upgrade or restart.
If the timeout exceeds and OSD is not ok to stop, then the operator would skip upgrade for the current OSD and proceed with the next one
if <code>continueUpgradeAfterChecksEvenIfNotHealthy</code> is <code>false</code>. If <code>continueUpgradeAfterChecksEvenIfNotHealthy</code> is <code>true</code>, then operator would
continue with the upgrade of an OSD even if its not ok to stop after the timeout. This timeout won&rsquo;t be applied if <code>skipUpgradeChecks</code> is <code>true</code>.
The default wait timeout is 10 minutes.</p>
</td>
</tr>
<tr>
<td>
<code>upgradeOSDRequiresHealthyPGs</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>UpgradeOSDRequiresHealthyPGs defines if OSD upgrade requires PGs are clean. If set to <code>true</code> OSD upgrade process won&rsquo;t start until PGs are healthy.
This configuration will be ignored if <code>skipUpgradeChecks</code> is <code>true</code>.
Default is false.</p>
</td>
</tr>
<tr>
<td>
<code>disruptionManagement</code><br/>
<em>
<a href="#ceph.rook.io/v1.DisruptionManagementSpec">
DisruptionManagementSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for configuring disruption management.</p>
</td>
</tr>
<tr>
<td>
<code>mon</code><br/>
<em>
<a href="#ceph.rook.io/v1.MonSpec">
MonSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for mon related options</p>
</td>
</tr>
<tr>
<td>
<code>crashCollector</code><br/>
<em>
<a href="#ceph.rook.io/v1.CrashCollectorSpec">
CrashCollectorSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for the crash controller</p>
</td>
</tr>
<tr>
<td>
<code>dashboard</code><br/>
<em>
<a href="#ceph.rook.io/v1.DashboardSpec">
DashboardSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Dashboard settings</p>
</td>
</tr>
<tr>
<td>
<code>monitoring</code><br/>
<em>
<a href="#ceph.rook.io/v1.MonitoringSpec">
MonitoringSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Prometheus based Monitoring settings</p>
</td>
</tr>
<tr>
<td>
<code>external</code><br/>
<em>
<a href="#ceph.rook.io/v1.ExternalSpec">
ExternalSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether the Ceph Cluster is running external to this Kubernetes cluster
mon, mgr, osd, mds, and discover daemons will not be created for external clusters.</p>
</td>
</tr>
<tr>
<td>
<code>mgr</code><br/>
<em>
<a href="#ceph.rook.io/v1.MgrSpec">
MgrSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A spec for mgr related options</p>
</td>
</tr>
<tr>
<td>
<code>removeOSDsIfOutAndSafeToRemove</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Remove the OSD that is out and safe to remove only if this option is true</p>
</td>
</tr>
<tr>
<td>
<code>cleanupPolicy</code><br/>
<em>
<a href="#ceph.rook.io/v1.CleanupPolicySpec">
CleanupPolicySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicates user intent when deleting a cluster; blocks orchestration and should not be set if cluster
deletion is not imminent.</p>
</td>
</tr>
<tr>
<td>
<code>healthCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephClusterHealthCheckSpec">
CephClusterHealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Internal daemon healthchecks and liveness probe</p>
</td>
</tr>
<tr>
<td>
<code>security</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterSecuritySpec">
ClusterSecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Security represents security settings</p>
</td>
</tr>
<tr>
<td>
<code>logCollector</code><br/>
<em>
<a href="#ceph.rook.io/v1.LogCollectorSpec">
LogCollectorSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Logging represents loggings settings</p>
</td>
</tr>
<tr>
<td>
<code>csi</code><br/>
<em>
<a href="#ceph.rook.io/v1.CSIDriverSpec">
CSIDriverSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>CSI Driver Options applied per cluster.</p>
</td>
</tr>
<tr>
<td>
<code>cephConfig</code><br/>
<em>
map[string]map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Ceph Config options</p>
</td>
</tr>
<tr>
<td>
<code>cephConfigFromSecret</code><br/>
<em>
map[string]map[string]k8s.io/api/core/v1.SecretKeySelector
</em>
</td>
<td>
<em>(Optional)</em>
<p>CephConfigFromSecret works exactly like CephConfig but takes config value from Secret Key reference.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ClusterState">ClusterState
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterStatus">ClusterStatus</a>)
</p>
<div>
<p>ClusterState represents the state of a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Connected&#34;</p></td>
<td><p>ClusterStateConnected represents the Connected state of a Ceph Cluster</p>
</td>
</tr><tr><td><p>&#34;Connecting&#34;</p></td>
<td><p>ClusterStateConnecting represents the Connecting state of a Ceph Cluster</p>
</td>
</tr><tr><td><p>&#34;Created&#34;</p></td>
<td><p>ClusterStateCreated represents the Created state of a Ceph Cluster</p>
</td>
</tr><tr><td><p>&#34;Creating&#34;</p></td>
<td><p>ClusterStateCreating represents the Creating state of a Ceph Cluster</p>
</td>
</tr><tr><td><p>&#34;Error&#34;</p></td>
<td><p>ClusterStateError represents the Error state of a Ceph Cluster</p>
</td>
</tr><tr><td><p>&#34;Updating&#34;</p></td>
<td><p>ClusterStateUpdating represents the Updating state of a Ceph Cluster</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.ClusterStatus">ClusterStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephCluster">CephCluster</a>)
</p>
<div>
<p>ClusterStatus represents the status of a Ceph cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>state</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterState">
ClusterState
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#ceph.rook.io/v1.Condition">
[]Condition
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>ceph</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephStatus">
CephStatus
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>storage</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephStorage">
CephStorage
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>version</code><br/>
<em>
<a href="#ceph.rook.io/v1.ClusterVersion">
ClusterVersion
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ClusterVersion">ClusterVersion
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterStatus">ClusterStatus</a>)
</p>
<div>
<p>ClusterVersion represents the version of a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>version</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CompressionSpec">CompressionSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ConnectionsSpec">ConnectionsSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to compress the data in transit across the wire.
The default is not set.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Condition">Condition
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">CephBlockPoolRadosNamespaceStatus</a>, <a href="#ceph.rook.io/v1.CephBlockPoolStatus">CephBlockPoolStatus</a>, <a href="#ceph.rook.io/v1.CephFilesystemStatus">CephFilesystemStatus</a>, <a href="#ceph.rook.io/v1.ClusterStatus">ClusterStatus</a>, <a href="#ceph.rook.io/v1.ObjectStoreStatus">ObjectStoreStatus</a>, <a href="#ceph.rook.io/v1.Status">Status</a>)
</p>
<div>
<p>Condition represents a status condition on any Rook-Ceph Custom Resource.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#conditionstatus-v1-core">
Kubernetes core/v1.ConditionStatus
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>reason</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionReason">
ConditionReason
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lastHeartbeatTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>lastTransitionTime</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#time-v1-meta">
Kubernetes meta/v1.Time
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ConditionReason">ConditionReason
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.Condition">Condition</a>)
</p>
<div>
<p>ConditionReason is a reason for a condition</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;ClusterConnected&#34;</p></td>
<td><p>ClusterConnectedReason is cluster connected reason</p>
</td>
</tr><tr><td><p>&#34;ClusterConnecting&#34;</p></td>
<td><p>ClusterConnectingReason is cluster connecting reason</p>
</td>
</tr><tr><td><p>&#34;ClusterCreated&#34;</p></td>
<td><p>ClusterCreatedReason is cluster created reason</p>
</td>
</tr><tr><td><p>&#34;ClusterDeleting&#34;</p></td>
<td><p>ClusterDeletingReason is cluster deleting reason</p>
</td>
</tr><tr><td><p>&#34;ClusterProgressing&#34;</p></td>
<td><p>ClusterProgressingReason is cluster progressing reason</p>
</td>
</tr><tr><td><p>&#34;Deleting&#34;</p></td>
<td><p>DeletingReason represents when Rook has detected a resource object should be deleted.</p>
</td>
</tr><tr><td><p>&#34;ObjectHasDependents&#34;</p></td>
<td><p>ObjectHasDependentsReason represents when a resource object has dependents that are blocking
deletion.</p>
</td>
</tr><tr><td><p>&#34;ObjectHasNoDependents&#34;</p></td>
<td><p>ObjectHasNoDependentsReason represents when a resource object has no dependents that are
blocking deletion.</p>
</td>
</tr><tr><td><p>&#34;PoolEmpty&#34;</p></td>
<td><p>PoolEmptyReason represents when a pool does not contain images or snapshots that are blocking
deletion.</p>
</td>
</tr><tr><td><p>&#34;PoolNotEmpty&#34;</p></td>
<td><p>PoolNotEmptyReason represents when a pool contains images or snapshots that are blocking
deletion.</p>
</td>
</tr><tr><td><p>&#34;RadosNamespaceEmpty&#34;</p></td>
<td><p>RadosNamespaceEmptyReason represents when a rados namespace does not contain images or snapshots that are blocking
deletion.</p>
</td>
</tr><tr><td><p>&#34;RadosNamespaceNotEmpty&#34;</p></td>
<td><p>RadosNamespaceNotEmptyReason represents when a rados namespace contains images or snapshots that are blocking
deletion.</p>
</td>
</tr><tr><td><p>&#34;ReconcileFailed&#34;</p></td>
<td><p>ReconcileFailed represents when a resource reconciliation failed.</p>
</td>
</tr><tr><td><p>&#34;ReconcileRequeuing&#34;</p></td>
<td><p>ReconcileRequeuing represents when a resource reconciliation requeue.</p>
</td>
</tr><tr><td><p>&#34;ReconcileStarted&#34;</p></td>
<td><p>ReconcileStarted represents when a resource reconciliation started.</p>
</td>
</tr><tr><td><p>&#34;ReconcileSucceeded&#34;</p></td>
<td><p>ReconcileSucceeded represents when a resource reconciliation was successful.</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.ConditionType">ConditionType
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">CephBlockPoolRadosNamespaceStatus</a>, <a href="#ceph.rook.io/v1.CephBlockPoolStatus">CephBlockPoolStatus</a>, <a href="#ceph.rook.io/v1.CephClientStatus">CephClientStatus</a>, <a href="#ceph.rook.io/v1.CephFilesystemStatus">CephFilesystemStatus</a>, <a href="#ceph.rook.io/v1.CephFilesystemSubVolumeGroupStatus">CephFilesystemSubVolumeGroupStatus</a>, <a href="#ceph.rook.io/v1.ClusterStatus">ClusterStatus</a>, <a href="#ceph.rook.io/v1.Condition">Condition</a>, <a href="#ceph.rook.io/v1.ObjectStoreStatus">ObjectStoreStatus</a>)
</p>
<div>
<p>ConditionType represent a resource&rsquo;s status</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;Connected&#34;</p></td>
<td><p>ConditionConnected represents Connected state of an object</p>
</td>
</tr><tr><td><p>&#34;Connecting&#34;</p></td>
<td><p>ConditionConnecting represents Connecting state of an object</p>
</td>
</tr><tr><td><p>&#34;Deleting&#34;</p></td>
<td><p>ConditionDeleting represents Deleting state of an object</p>
</td>
</tr><tr><td><p>&#34;DeletionIsBlocked&#34;</p></td>
<td><p>ConditionDeletionIsBlocked represents when deletion of the object is blocked.</p>
</td>
</tr><tr><td><p>&#34;Failure&#34;</p></td>
<td><p>ConditionFailure represents Failure state of an object</p>
</td>
</tr><tr><td><p>&#34;PoolDeletionIsBlocked&#34;</p></td>
<td><p>ConditionPoolDeletionIsBlocked represents when deletion of the object is blocked.</p>
</td>
</tr><tr><td><p>&#34;Progressing&#34;</p></td>
<td><p>ConditionProgressing represents Progressing state of an object</p>
</td>
</tr><tr><td><p>&#34;RadosNamespaceDeletionIsBlocked&#34;</p></td>
<td><p>ConditionRadosNSDeletionIsBlocked represents when deletion of the object is blocked.</p>
</td>
</tr><tr><td><p>&#34;Ready&#34;</p></td>
<td><p>ConditionReady represents Ready state of an object</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.ConfigFileVolumeSource">ConfigFileVolumeSource
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.AdditionalVolumeMount">AdditionalVolumeMount</a>, <a href="#ceph.rook.io/v1.KerberosConfigFiles">KerberosConfigFiles</a>, <a href="#ceph.rook.io/v1.KerberosKeytabFile">KerberosKeytabFile</a>, <a href="#ceph.rook.io/v1.SSSDSidecarConfigFile">SSSDSidecarConfigFile</a>)
</p>
<div>
<p>Represents the source of a volume to mount.
Only one of its members may be specified.
This is a subset of the full Kubernetes API&rsquo;s VolumeSource that is reduced to what is most likely
to be useful for mounting config files/dirs into Rook pods.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>hostPath</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#hostpathvolumesource-v1-core">
Kubernetes core/v1.HostPathVolumeSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>hostPath represents a pre-existing file or directory on the host
machine that is directly exposed to the container. This is generally
used for system agents or other privileged things that are allowed
to see the host machine. Most containers will NOT need this.
More info: <a href="https://kubernetes.io/docs/concepts/storage/volumes#hostpath">https://kubernetes.io/docs/concepts/storage/volumes#hostpath</a></p>
<hr />
</td>
</tr>
<tr>
<td>
<code>emptyDir</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#emptydirvolumesource-v1-core">
Kubernetes core/v1.EmptyDirVolumeSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>emptyDir represents a temporary directory that shares a pod&rsquo;s lifetime.
More info: <a href="https://kubernetes.io/docs/concepts/storage/volumes#emptydir">https://kubernetes.io/docs/concepts/storage/volumes#emptydir</a></p>
</td>
</tr>
<tr>
<td>
<code>secret</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretvolumesource-v1-core">
Kubernetes core/v1.SecretVolumeSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>secret represents a secret that should populate this volume.
More info: <a href="https://kubernetes.io/docs/concepts/storage/volumes#secret">https://kubernetes.io/docs/concepts/storage/volumes#secret</a></p>
</td>
</tr>
<tr>
<td>
<code>persistentVolumeClaim</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#persistentvolumeclaimvolumesource-v1-core">
Kubernetes core/v1.PersistentVolumeClaimVolumeSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>persistentVolumeClaimVolumeSource represents a reference to a
PersistentVolumeClaim in the same namespace.
More info: <a href="https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims">https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims</a></p>
</td>
</tr>
<tr>
<td>
<code>configMap</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#configmapvolumesource-v1-core">
Kubernetes core/v1.ConfigMapVolumeSource
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>configMap represents a configMap that should populate this volume</p>
</td>
</tr>
<tr>
<td>
<code>projected</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#projectedvolumesource-v1-core">
Kubernetes core/v1.ProjectedVolumeSource
</a>
</em>
</td>
<td>
<p>projected items for all in one resources secrets, configmaps, and downward API</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ConnectionsSpec">ConnectionsSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NetworkSpec">NetworkSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>encryption</code><br/>
<em>
<a href="#ceph.rook.io/v1.EncryptionSpec">
EncryptionSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Encryption settings for the network connections.</p>
</td>
</tr>
<tr>
<td>
<code>compression</code><br/>
<em>
<a href="#ceph.rook.io/v1.CompressionSpec">
CompressionSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Compression settings for the network connections.</p>
</td>
</tr>
<tr>
<td>
<code>requireMsgr2</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to require msgr2 (port 3300) even if compression or encryption are not enabled.
If true, the msgr1 port (6789) will be disabled.
Requires a kernel that supports msgr2 (kernel 5.11 or CentOS 8.4 or newer).</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.CrashCollectorSpec">CrashCollectorSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>CrashCollectorSpec represents options to configure the crash controller</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>disable</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disable determines whether we should enable the crash collector</p>
</td>
</tr>
<tr>
<td>
<code>daysToRetain</code><br/>
<em>
uint
</em>
</td>
<td>
<em>(Optional)</em>
<p>DaysToRetain represents the number of days to retain crash until they get pruned</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.DaemonHealthSpec">DaemonHealthSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephClusterHealthCheckSpec">CephClusterHealthCheckSpec</a>)
</p>
<div>
<p>DaemonHealthSpec is a daemon health check</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#ceph.rook.io/v1.HealthCheckSpec">
HealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Status represents the health check settings for the Ceph health</p>
</td>
</tr>
<tr>
<td>
<code>mon</code><br/>
<em>
<a href="#ceph.rook.io/v1.HealthCheckSpec">
HealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Monitor represents the health check settings for the Ceph monitor</p>
</td>
</tr>
<tr>
<td>
<code>osd</code><br/>
<em>
<a href="#ceph.rook.io/v1.HealthCheckSpec">
HealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObjectStorageDaemon represents the health check settings for the Ceph OSDs</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.DashboardSpec">DashboardSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>DashboardSpec represents the settings for the Ceph dashboard</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled determines whether to enable the dashboard</p>
</td>
</tr>
<tr>
<td>
<code>urlPrefix</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>URLPrefix is a prefix for all URLs to use the dashboard with a reverse proxy</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Port is the dashboard webserver port</p>
</td>
</tr>
<tr>
<td>
<code>ssl</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>SSL determines whether SSL should be used</p>
</td>
</tr>
<tr>
<td>
<code>prometheusEndpoint</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Endpoint for the Prometheus host</p>
</td>
</tr>
<tr>
<td>
<code>prometheusEndpointSSLVerify</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to verify the ssl endpoint for prometheus. Set to false for a self-signed cert.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Device">Device
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.Selection">Selection</a>)
</p>
<div>
<p>Device represents a disk to use in the cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>fullpath</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>config</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.DeviceClasses">DeviceClasses
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephStorage">CephStorage</a>)
</p>
<div>
<p>DeviceClasses represents device classes of a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.DisruptionManagementSpec">DisruptionManagementSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>DisruptionManagementSpec configures management of daemon disruptions</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>managePodBudgets</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>This enables management of poddisruptionbudgets</p>
</td>
</tr>
<tr>
<td>
<code>osdMaintenanceTimeout</code><br/>
<em>
time.Duration
</em>
</td>
<td>
<em>(Optional)</em>
<p>OSDMaintenanceTimeout sets how many additional minutes the DOWN/OUT interval is for drained failure domains
it only works if managePodBudgets is true.
the default is 30 minutes</p>
</td>
</tr>
<tr>
<td>
<code>pgHealthCheckTimeout</code><br/>
<em>
time.Duration
</em>
</td>
<td>
<em>(Optional)</em>
<p>DEPRECATED: PGHealthCheckTimeout is no longer implemented</p>
</td>
</tr>
<tr>
<td>
<code>pgHealthyRegex</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PgHealthyRegex is the regular expression that is used to determine which PG states should be considered healthy.
The default is <code>^(active\+clean|active\+clean\+scrubbing|active\+clean\+scrubbing\+deep)$</code></p>
</td>
</tr>
<tr>
<td>
<code>manageMachineDisruptionBudgets</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Deprecated. This enables management of machinedisruptionbudgets.</p>
</td>
</tr>
<tr>
<td>
<code>machineDisruptionBudgetNamespace</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Deprecated. Namespace to look for MDBs by the machineDisruptionBudgetController</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.EncryptionSpec">EncryptionSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ConnectionsSpec">ConnectionsSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to encrypt the data in transit across the wire to prevent eavesdropping
the data on the network. The default is not set. Even if encryption is not enabled,
clients still establish a strong initial authentication for the connection
and data integrity is still validated with a crc check. When encryption is enabled,
all communication between clients and Ceph daemons, or between Ceph daemons will
be encrypted.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.EndpointAddress">EndpointAddress
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>)
</p>
<div>
<p>EndpointAddress is a tuple that describes a single IP address or host name. This is a subset of
Kubernetes&rsquo;s v1.EndpointAddress.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ip</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The IP of this endpoint. As a legacy behavior, this supports being given a DNS-addressable hostname as well.</p>
</td>
</tr>
<tr>
<td>
<code>hostname</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The DNS-addressable Hostname of this endpoint. This field will be preferred over IP if both are given.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ErasureCodedSpec">ErasureCodedSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.PoolSpec">PoolSpec</a>)
</p>
<div>
<p>ErasureCodedSpec represents the spec for erasure code in a pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>codingChunks</code><br/>
<em>
uint
</em>
</td>
<td>
<p>Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type).
This is the number of OSDs that can be lost simultaneously before data cannot be recovered.</p>
</td>
</tr>
<tr>
<td>
<code>dataChunks</code><br/>
<em>
uint
</em>
</td>
<td>
<p>Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type).
The number of chunks required to recover an object when any single OSD is lost is the same
as dataChunks so be aware that the larger the number of data chunks, the higher the cost of recovery.</p>
</td>
</tr>
<tr>
<td>
<code>algorithm</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The algorithm for erasure coding</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ExternalSpec">ExternalSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>ExternalSpec represents the options supported by an external cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enable</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enable determines whether external mode is enabled or not</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FSMirroringSpec">FSMirroringSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemSpec">FilesystemSpec</a>)
</p>
<div>
<p>FSMirroringSpec represents the setting for a mirrored filesystem</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled whether this filesystem is mirrored or not</p>
</td>
</tr>
<tr>
<td>
<code>peers</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringPeerSpec">
MirroringPeerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Peers represents the peers spec</p>
</td>
</tr>
<tr>
<td>
<code>snapshotSchedules</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotScheduleSpec">
[]SnapshotScheduleSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SnapshotSchedules is the scheduling of snapshot for mirrored filesystems</p>
</td>
</tr>
<tr>
<td>
<code>snapshotRetention</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotScheduleRetentionSpec">
[]SnapshotScheduleRetentionSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Retention is the retention policy for a snapshot schedule
One path has exactly one retention policy.
A policy can however contain multiple count-time period pairs in order to specify complex retention policies</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemMirrorInfoPeerSpec">FilesystemMirrorInfoPeerSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemsSpec">FilesystemsSpec</a>)
</p>
<div>
<p>FilesystemMirrorInfoPeerSpec is the specification of a filesystem peer mirror</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uuid</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>UUID is the peer unique identifier</p>
</td>
</tr>
<tr>
<td>
<code>remote</code><br/>
<em>
<a href="#ceph.rook.io/v1.PeerRemoteSpec">
PeerRemoteSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Remote are the remote cluster information</p>
</td>
</tr>
<tr>
<td>
<code>stats</code><br/>
<em>
<a href="#ceph.rook.io/v1.PeerStatSpec">
PeerStatSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Stats are the stat a peer mirror</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemMirroringInfo">FilesystemMirroringInfo
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemMirroringInfoSpec">FilesystemMirroringInfoSpec</a>)
</p>
<div>
<p>FilesystemMirrorInfoSpec is the filesystem mirror status of a given filesystem</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>daemon_id</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>DaemonID is the cephfs-mirror name</p>
</td>
</tr>
<tr>
<td>
<code>filesystems</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemsSpec">
[]FilesystemsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Filesystems is the list of filesystems managed by a given cephfs-mirror daemon</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemMirroringInfoSpec">FilesystemMirroringInfoSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystemStatus">CephFilesystemStatus</a>)
</p>
<div>
<p>FilesystemMirroringInfo is the status of the pool mirroring</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>daemonsStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemMirroringInfo">
[]FilesystemMirroringInfo
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PoolMirroringStatus is the mirroring status of a filesystem</p>
</td>
</tr>
<tr>
<td>
<code>lastChecked</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChecked is the last time time the status was checked</p>
</td>
</tr>
<tr>
<td>
<code>lastChanged</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChanged is the last time time the status last changed</p>
</td>
</tr>
<tr>
<td>
<code>details</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Details contains potential status errors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemMirroringSpec">FilesystemMirroringSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystemMirror">CephFilesystemMirror</a>)
</p>
<div>
<p>FilesystemMirroringSpec is the filesystem mirroring specification</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the rgw pods (default is to place on any available node)</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The resource requirements for the cephfs-mirror pods</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets priority class on the cephfs-mirror pods</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemSnapshotScheduleStatusRetention">FilesystemSnapshotScheduleStatusRetention
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemSnapshotSchedulesSpec">FilesystemSnapshotSchedulesSpec</a>)
</p>
<div>
<p>FilesystemSnapshotScheduleStatusRetention is the retention specification for a filesystem snapshot schedule</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>start</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Start is when the snapshot schedule starts</p>
</td>
</tr>
<tr>
<td>
<code>created</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Created is when the snapshot schedule was created</p>
</td>
</tr>
<tr>
<td>
<code>first</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>First is when the first snapshot schedule was taken</p>
</td>
</tr>
<tr>
<td>
<code>last</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Last is when the last snapshot schedule was taken</p>
</td>
</tr>
<tr>
<td>
<code>last_pruned</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastPruned is when the last snapshot schedule was pruned</p>
</td>
</tr>
<tr>
<td>
<code>created_count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>CreatedCount is total amount of snapshots</p>
</td>
</tr>
<tr>
<td>
<code>pruned_count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>PrunedCount is total amount of pruned snapshots</p>
</td>
</tr>
<tr>
<td>
<code>active</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Active is whether the scheduled is active or not</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemSnapshotScheduleStatusSpec">FilesystemSnapshotScheduleStatusSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystemStatus">CephFilesystemStatus</a>)
</p>
<div>
<p>FilesystemSnapshotScheduleStatusSpec is the status of the snapshot schedule</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>snapshotSchedules</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemSnapshotSchedulesSpec">
[]FilesystemSnapshotSchedulesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SnapshotSchedules is the list of snapshots scheduled</p>
</td>
</tr>
<tr>
<td>
<code>lastChecked</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChecked is the last time time the status was checked</p>
</td>
</tr>
<tr>
<td>
<code>lastChanged</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChanged is the last time time the status last changed</p>
</td>
</tr>
<tr>
<td>
<code>details</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Details contains potential status errors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemSnapshotSchedulesSpec">FilesystemSnapshotSchedulesSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemSnapshotScheduleStatusSpec">FilesystemSnapshotScheduleStatusSpec</a>)
</p>
<div>
<p>FilesystemSnapshotSchedulesSpec is the list of snapshot scheduled for images in a pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>fs</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Fs is the name of the Ceph Filesystem</p>
</td>
</tr>
<tr>
<td>
<code>subvol</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Subvol is the name of the sub volume</p>
</td>
</tr>
<tr>
<td>
<code>path</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path is the path on the filesystem</p>
</td>
</tr>
<tr>
<td>
<code>rel_path</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>schedule</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>retention</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemSnapshotScheduleStatusRetention">
FilesystemSnapshotScheduleStatusRetention
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemSpec">FilesystemSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephFilesystem">CephFilesystem</a>)
</p>
<div>
<p>FilesystemSpec represents the spec of a file system</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.NamedPoolSpec">
NamedPoolSpec
</a>
</em>
</td>
<td>
<p>The metadata pool settings</p>
</td>
</tr>
<tr>
<td>
<code>dataPools</code><br/>
<em>
<a href="#ceph.rook.io/v1.NamedPoolSpec">
[]NamedPoolSpec
</a>
</em>
</td>
<td>
<p>The data pool settings, with optional predefined pool name.</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolNames</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pool names as specified</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolsOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pools on filesystem deletion</p>
</td>
</tr>
<tr>
<td>
<code>preserveFilesystemOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve the fs in the cluster on CephFilesystem CR deletion. Setting this to true automatically implies PreservePoolsOnDelete is true.</p>
</td>
</tr>
<tr>
<td>
<code>metadataServer</code><br/>
<em>
<a href="#ceph.rook.io/v1.MetadataServerSpec">
MetadataServerSpec
</a>
</em>
</td>
<td>
<p>The mds pod info</p>
</td>
</tr>
<tr>
<td>
<code>mirroring</code><br/>
<em>
<a href="#ceph.rook.io/v1.FSMirroringSpec">
FSMirroringSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The mirroring settings</p>
</td>
</tr>
<tr>
<td>
<code>statusCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirrorHealthCheckSpec">
MirrorHealthCheckSpec
</a>
</em>
</td>
<td>
<p>The mirroring statusCheck</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.FilesystemsSpec">FilesystemsSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemMirroringInfo">FilesystemMirroringInfo</a>)
</p>
<div>
<p>FilesystemsSpec is spec for the mirrored filesystem</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>filesystem_id</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>FilesystemID is the filesystem identifier</p>
</td>
</tr>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name is name of the filesystem</p>
</td>
</tr>
<tr>
<td>
<code>directory_count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>DirectoryCount is the number of directories in the filesystem</p>
</td>
</tr>
<tr>
<td>
<code>peers</code><br/>
<em>
<a href="#ceph.rook.io/v1.FilesystemMirrorInfoPeerSpec">
[]FilesystemMirrorInfoPeerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Peers represents the mirroring peers</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.GaneshaRADOSSpec">GaneshaRADOSSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NFSGaneshaSpec">NFSGaneshaSpec</a>)
</p>
<div>
<p>GaneshaRADOSSpec represents the specification of a Ganesha RADOS object</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>pool</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The Ceph pool used store the shared configuration for NFS-Ganesha daemons.
This setting is deprecated, as it is internally required to be &ldquo;.nfs&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The namespace inside the Ceph pool (set by &lsquo;pool&rsquo;) where shared NFS-Ganesha config is stored.
This setting is deprecated as it is internally set to the name of the CephNFS.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.GaneshaServerSpec">GaneshaServerSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NFSGaneshaSpec">NFSGaneshaSpec</a>)
</p>
<div>
<p>GaneshaServerSpec represents the specification of a Ganesha Server</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>active</code><br/>
<em>
int
</em>
</td>
<td>
<p>The number of active Ganesha servers</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the ganesha pods</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources set resource requests and limits</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets the priority class on the pods</p>
</td>
</tr>
<tr>
<td>
<code>logLevel</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LogLevel set logging level</p>
</td>
</tr>
<tr>
<td>
<code>hostNetwork</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether host networking is enabled for the Ganesha server. If not set, the network settings from the cluster CR will be applied.</p>
</td>
</tr>
<tr>
<td>
<code>livenessProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProbeSpec">
ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>A liveness-probe to verify that Ganesha server has valid run-time state.
If LivenessProbe.Disabled is false and LivenessProbe.Probe is nil uses default probe.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.GatewaySpec">GatewaySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>GatewaySpec represents the specification of Ceph Object Store Gateway</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>port</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The port the rgw service will be listening on (http)</p>
</td>
</tr>
<tr>
<td>
<code>securePort</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The port the rgw service will be listening on (https)</p>
</td>
</tr>
<tr>
<td>
<code>instances</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The number of pods in the rgw replicaset.</p>
</td>
</tr>
<tr>
<td>
<code>sslCertificateRef</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the secret that stores the ssl certificate for secure rgw connections</p>
</td>
</tr>
<tr>
<td>
<code>caBundleRef</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the secret that stores custom ca-bundle with root and intermediate certificates.</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the rgw pods (default is to place on any available node)</p>
</td>
</tr>
<tr>
<td>
<code>disableMultisiteSyncTraffic</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DisableMultisiteSyncTraffic, when true, prevents this object store&rsquo;s gateways from
transmitting multisite replication data. Note that this value does not affect whether
gateways receive multisite replication traffic: see ObjectZone.spec.customEndpoints for that.
If false or unset, this object store&rsquo;s gateways will be able to transmit multisite
replication data.</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The resource requirements for the rgw pods</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets priority classes on the rgw pods</p>
</td>
</tr>
<tr>
<td>
<code>externalRgwEndpoints</code><br/>
<em>
<a href="#ceph.rook.io/v1.EndpointAddress">
[]EndpointAddress
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ExternalRgwEndpoints points to external RGW endpoint(s). Multiple endpoints can be given, but
for stability of ObjectBucketClaims, we highly recommend that users give only a single
external RGW endpoint that is a load balancer that sends requests to the multiple RGWs.</p>
</td>
</tr>
<tr>
<td>
<code>service</code><br/>
<em>
<a href="#ceph.rook.io/v1.RGWServiceSpec">
RGWServiceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The configuration related to add/set on each rgw service.</p>
</td>
</tr>
<tr>
<td>
<code>opsLogSidecar</code><br/>
<em>
<a href="#ceph.rook.io/v1.OpsLogSidecar">
OpsLogSidecar
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enable enhanced operation Logs for S3 in a sidecar named ops-log</p>
</td>
</tr>
<tr>
<td>
<code>hostNetwork</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether host networking is enabled for the rgw daemon. If not set, the network settings from the cluster CR will be applied.</p>
</td>
</tr>
<tr>
<td>
<code>dashboardEnabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether rgw dashboard is enabled for the rgw daemon. If not set, the rgw dashboard will be enabled.</p>
</td>
</tr>
<tr>
<td>
<code>additionalVolumeMounts</code><br/>
<em>
<a href="#ceph.rook.io/v1.AdditionalVolumeMounts">
AdditionalVolumeMounts
</a>
</em>
</td>
<td>
<p>AdditionalVolumeMounts allows additional volumes to be mounted to the RGW pod.
The root directory for each additional volume mount is <code>/var/rgw</code>.
Example: for an additional mount at subPath <code>ldap</code>, mounted from a secret that has key
<code>bindpass.secret</code>, the file would reside at <code>/var/rgw/ldap/bindpass.secret</code>.</p>
</td>
</tr>
<tr>
<td>
<code>rgwConfig</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>RgwConfig sets Ceph RGW config values for the gateway clients that serve this object store.
Values are modified at runtime without RGW restart.
This feature is intended for advanced users. It allows breaking configurations to be easily
applied. Use with caution.</p>
</td>
</tr>
<tr>
<td>
<code>rgwConfigFromSecret</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
map[string]k8s.io/api/core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RgwConfigFromSecret works exactly like RgwConfig but takes config value from Secret Key reference.
Values are modified at runtime without RGW restart.
This feature is intended for advanced users. It allows breaking configurations to be easily
applied. Use with caution.</p>
</td>
</tr>
<tr>
<td>
<code>rgwCommandFlags</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>RgwCommandFlags sets Ceph RGW config values for the gateway clients that serve this object
store. Values are modified at RGW startup, resulting in RGW pod restarts.
This feature is intended for advanced users. It allows breaking configurations to be easily
applied. Use with caution.</p>
</td>
</tr>
<tr>
<td>
<code>readAffinity</code><br/>
<em>
<a href="#ceph.rook.io/v1.RgwReadAffinity">
RgwReadAffinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReadAffinity defines the RGW read affinity policy to optimize the read requests for the RGW clients
Note: Only supported from Ceph Tentacle (v20)</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.HTTPEndpointSpec">HTTPEndpointSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.TopicEndpointSpec">TopicEndpointSpec</a>)
</p>
<div>
<p>HTTPEndpointSpec represent the spec of an HTTP endpoint of a Bucket Topic</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uri</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URI of the HTTP endpoint to push notification to</p>
</td>
</tr>
<tr>
<td>
<code>disableVerifySSL</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicate whether the server certificate is validated by the client or not</p>
</td>
</tr>
<tr>
<td>
<code>sendCloudEvents</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Send the notifications with the CloudEvents header: <a href="https://github.com/cloudevents/spec/blob/main/cloudevents/adapters/aws-s3.md">https://github.com/cloudevents/spec/blob/main/cloudevents/adapters/aws-s3.md</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.HealthCheckSpec">HealthCheckSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.DaemonHealthSpec">DaemonHealthSpec</a>, <a href="#ceph.rook.io/v1.MirrorHealthCheckSpec">MirrorHealthCheckSpec</a>)
</p>
<div>
<p>HealthCheckSpec represents the health check of an object store bucket</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>disabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>interval</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Interval is the internal in second or minute for the health check to run like 60s for 60 seconds</p>
</td>
</tr>
<tr>
<td>
<code>timeout</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.HybridStorageSpec">HybridStorageSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ReplicatedSpec">ReplicatedSpec</a>)
</p>
<div>
<p>HybridStorageSpec represents the settings for hybrid storage pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>primaryDeviceClass</code><br/>
<em>
string
</em>
</td>
<td>
<p>PrimaryDeviceClass represents high performance tier (for example SSD or NVME) for Primary OSD</p>
</td>
</tr>
<tr>
<td>
<code>secondaryDeviceClass</code><br/>
<em>
string
</em>
</td>
<td>
<p>SecondaryDeviceClass represents low performance tier (for example HDDs) for remaining OSDs</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.IPFamilyType">IPFamilyType
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NetworkSpec">NetworkSpec</a>)
</p>
<div>
<p>IPFamilyType represents the single stack Ipv4 or Ipv6 protocol.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;IPv4&#34;</p></td>
<td><p>IPv4 internet protocol version</p>
</td>
</tr><tr><td><p>&#34;IPv6&#34;</p></td>
<td><p>IPv6 internet protocol version</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.ImplicitTenantSetting">ImplicitTenantSetting
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.KeystoneSpec">KeystoneSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;false&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;s3&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;swift&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;true&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.KafkaEndpointSpec">KafkaEndpointSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.TopicEndpointSpec">TopicEndpointSpec</a>)
</p>
<div>
<p>KafkaEndpointSpec represent the spec of a Kafka endpoint of a Bucket Topic</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uri</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URI of the Kafka endpoint to push notification to</p>
</td>
</tr>
<tr>
<td>
<code>useSSL</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicate whether to use SSL when communicating with the broker</p>
</td>
</tr>
<tr>
<td>
<code>disableVerifySSL</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Indicate whether the server certificate is validated by the client or not</p>
</td>
</tr>
<tr>
<td>
<code>ackLevel</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The ack level required for this topic (none/broker)</p>
</td>
</tr>
<tr>
<td>
<code>mechanism</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The authentication mechanism for this topic (PLAIN/SCRAM-SHA-512/SCRAM-SHA-256/GSSAPI/OAUTHBEARER)</p>
</td>
</tr>
<tr>
<td>
<code>userSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The kafka user name to use for authentication</p>
</td>
</tr>
<tr>
<td>
<code>passwordSecretRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The kafka password to use for authentication</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.KerberosConfigFiles">KerberosConfigFiles
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.KerberosSpec">KerberosSpec</a>)
</p>
<div>
<p>KerberosConfigFiles represents the source(s) from which Kerberos configuration should come.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>volumeSource</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConfigFileVolumeSource">
ConfigFileVolumeSource
</a>
</em>
</td>
<td>
<p>VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for
Kerberos configuration files like what is normally used to configure Volumes for a Pod. For
example, a ConfigMap, Secret, or HostPath. The volume may contain multiple files, all of
which will be loaded.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.KerberosKeytabFile">KerberosKeytabFile
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.KerberosSpec">KerberosSpec</a>)
</p>
<div>
<p>KerberosKeytabFile represents the source(s) from which the Kerberos keytab file should come.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>volumeSource</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConfigFileVolumeSource">
ConfigFileVolumeSource
</a>
</em>
</td>
<td>
<p>VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for the
Kerberos keytab file like what is normally used to configure Volumes for a Pod. For example,
a Secret or HostPath.
There are two requirements for the source&rsquo;s content:
1. The config file must be mountable via <code>subPath: krb5.keytab</code>. For example, in a
Secret, the data item must be named <code>krb5.keytab</code>, or <code>items</code> must be defined to
select the key and give it path <code>krb5.keytab</code>. A HostPath directory must have the
<code>krb5.keytab</code> file.
2. The volume or config file must have mode 0600.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.KerberosSpec">KerberosSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NFSSecuritySpec">NFSSecuritySpec</a>)
</p>
<div>
<p>KerberosSpec represents configuration for Kerberos.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>principalName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PrincipalName corresponds directly to NFS-Ganesha&rsquo;s NFS_KRB5:PrincipalName config. In
practice, this is the service prefix of the principal name. The default is &ldquo;nfs&rdquo;.
This value is combined with (a) the namespace and name of the CephNFS (with a hyphen between)
and (b) the Realm configured in the user-provided krb5.conf to determine the full principal
name: <principalName>/<namespace>-<name>@<realm>. e.g., nfs/rook-ceph-my-nfs@example.net.
See <a href="https://github.com/nfs-ganesha/nfs-ganesha/wiki/RPCSEC_GSS">https://github.com/nfs-ganesha/nfs-ganesha/wiki/RPCSEC_GSS</a> for more detail.</p>
</td>
</tr>
<tr>
<td>
<code>domainName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>DomainName should be set to the Kerberos Realm.</p>
</td>
</tr>
<tr>
<td>
<code>configFiles</code><br/>
<em>
<a href="#ceph.rook.io/v1.KerberosConfigFiles">
KerberosConfigFiles
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConfigFiles defines where the Kerberos configuration should be sourced from. Config files
will be placed into the <code>/etc/krb5.conf.rook/</code> directory.</p>
<p>If this is left empty, Rook will not add any files. This allows you to manage the files
yourself however you wish. For example, you may build them into your custom Ceph container
image or use the Vault agent injector to securely add the files via annotations on the
CephNFS spec (passed to the NFS server pods).</p>
<p>Rook configures Kerberos to log to stderr. We suggest removing logging sections from config
files to avoid consuming unnecessary disk space from logging to files.</p>
</td>
</tr>
<tr>
<td>
<code>keytabFile</code><br/>
<em>
<a href="#ceph.rook.io/v1.KerberosKeytabFile">
KerberosKeytabFile
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeytabFile defines where the Kerberos keytab should be sourced from. The keytab file will be
placed into <code>/etc/krb5.keytab</code>. If this is left empty, Rook will not add the file.
This allows you to manage the <code>krb5.keytab</code> file yourself however you wish. For example, you
may build it into your custom Ceph container image or use the Vault agent injector to
securely add the file via annotations on the CephNFS spec (passed to the NFS server pods).</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.KeyManagementServiceSpec">KeyManagementServiceSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSecuritySpec">ClusterSecuritySpec</a>, <a href="#ceph.rook.io/v1.ObjectStoreSecuritySpec">ObjectStoreSecuritySpec</a>, <a href="#ceph.rook.io/v1.SecuritySpec">SecuritySpec</a>)
</p>
<div>
<p>KeyManagementServiceSpec represent various details of the KMS server</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>connectionDetails</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConnectionDetails contains the KMS connection details (address, port etc)</p>
</td>
</tr>
<tr>
<td>
<code>tokenSecretName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>TokenSecretName is the kubernetes secret containing the KMS token</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.KeyRotationSpec">KeyRotationSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSecuritySpec">ClusterSecuritySpec</a>, <a href="#ceph.rook.io/v1.SecuritySpec">SecuritySpec</a>)
</p>
<div>
<p>KeyRotationSpec represents the settings for Key Rotation.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled represents whether the key rotation is enabled.</p>
</td>
</tr>
<tr>
<td>
<code>schedule</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Schedule represents the cron schedule for key rotation.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.KeyType">KeyType
(<code>string</code> alias)</h3>
<div>
<p>KeyType type safety</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;exporter&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;cleanup&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;clusterMetadata&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;cmdreporter&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;crashcollector&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;dashboard&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;mds&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;mgr&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;mon&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;arbiter&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;monitoring&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;osd&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;prepareosd&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;rgw&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;keyrotation&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.KeystoneSpec">KeystoneSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.AuthSpec">AuthSpec</a>)
</p>
<div>
<p>KeystoneSpec represents the Keystone authentication configuration of a Ceph Object Store Gateway</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>url</code><br/>
<em>
string
</em>
</td>
<td>
<p>The URL for the Keystone server.</p>
</td>
</tr>
<tr>
<td>
<code>serviceUserSecretName</code><br/>
<em>
string
</em>
</td>
<td>
<p>The name of the secret containing the credentials for the service user account used by RGW. It has to be in the same namespace as the object store resource.</p>
</td>
</tr>
<tr>
<td>
<code>acceptedRoles</code><br/>
<em>
[]string
</em>
</td>
<td>
<p>The roles requires to serve requests.</p>
</td>
</tr>
<tr>
<td>
<code>implicitTenants</code><br/>
<em>
<a href="#ceph.rook.io/v1.ImplicitTenantSetting">
ImplicitTenantSetting
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Create new users in their own tenants of the same name. Possible values are true, false, swift and s3. The latter have the effect of splitting the identity space such that only the indicated protocol will use implicit tenants.</p>
</td>
</tr>
<tr>
<td>
<code>tokenCacheSize</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>The maximum number of entries in each Keystone token cache.</p>
</td>
</tr>
<tr>
<td>
<code>revocationInterval</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>The number of seconds between token revocation checks.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Labels">Labels
(<code>map[string]string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemMirroringSpec">FilesystemMirroringSpec</a>, <a href="#ceph.rook.io/v1.GaneshaServerSpec">GaneshaServerSpec</a>, <a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>, <a href="#ceph.rook.io/v1.MetadataServerSpec">MetadataServerSpec</a>, <a href="#ceph.rook.io/v1.RBDMirroringSpec">RBDMirroringSpec</a>)
</p>
<div>
<p>Labels are label for a given daemons</p>
</div>
<h3 id="ceph.rook.io/v1.LabelsSpec">LabelsSpec
(<code>map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.KeyType]github.com/rook/rook/pkg/apis/ceph.rook.io/v1.Labels</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>LabelsSpec is the main spec label for all daemons</p>
</div>
<h3 id="ceph.rook.io/v1.LocalCephxStatus">LocalCephxStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreStatus">ObjectStoreStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>daemon</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephxStatus">
CephxStatus
</a>
</em>
</td>
<td>
<p>Daemon shows the CephX key status for local Ceph daemons associated with this resources.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.LogCollectorSpec">LogCollectorSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>LogCollectorSpec is the logging spec</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled represents whether the log collector is enabled</p>
</td>
</tr>
<tr>
<td>
<code>periodicity</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Periodicity is the periodicity of the log rotation.</p>
</td>
</tr>
<tr>
<td>
<code>maxLogSize</code><br/>
<em>
k8s.io/apimachinery/pkg/api/resource.Quantity
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxLogSize is the maximum size of the log per ceph daemons. Must be at least 1M.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MetadataServerSpec">MetadataServerSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemSpec">FilesystemSpec</a>)
</p>
<div>
<p>MetadataServerSpec represents the specification of a Ceph Metadata Server</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>activeCount</code><br/>
<em>
int32
</em>
</td>
<td>
<p>The number of metadata servers that are active. The remaining servers in the cluster will be in standby mode.</p>
</td>
</tr>
<tr>
<td>
<code>activeStandby</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether each active MDS instance will have an active standby with a warm metadata cache for faster failover.
If false, standbys will still be available, but will not have a warm metadata cache.</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the mds pods (default is to place on all available node) with a daemonset</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The resource requirements for the mds pods</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets priority classes on components</p>
</td>
</tr>
<tr>
<td>
<code>livenessProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProbeSpec">
ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>startupProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProbeSpec">
ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MgrSpec">MgrSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>MgrSpec represents options to configure a ceph mgr</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Count is the number of manager daemons to run</p>
</td>
</tr>
<tr>
<td>
<code>allowMultiplePerNode</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AllowMultiplePerNode allows to run multiple managers on the same node (not recommended)</p>
</td>
</tr>
<tr>
<td>
<code>modules</code><br/>
<em>
<a href="#ceph.rook.io/v1.Module">
[]Module
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Modules is the list of ceph manager modules to enable/disable</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Migration">Migration
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.StorageScopeSpec">StorageScopeSpec</a>)
</p>
<div>
<p>Migration handles the OSD migration</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>confirmation</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>A user confirmation to migrate the OSDs. It destroys each OSD one at a time, cleans up the backing disk
and prepares OSD with same ID on that disk</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MigrationStatus">MigrationStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.OSDStatus">OSDStatus</a>)
</p>
<div>
<p>MigrationStatus status represents the current status of any OSD migration.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>pending</code><br/>
<em>
int
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirrorHealthCheckSpec">MirrorHealthCheckSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemSpec">FilesystemSpec</a>, <a href="#ceph.rook.io/v1.PoolSpec">PoolSpec</a>)
</p>
<div>
<p>MirrorHealthCheckSpec represents the health specification of a Ceph Storage Pool mirror</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>mirror</code><br/>
<em>
<a href="#ceph.rook.io/v1.HealthCheckSpec">
HealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringInfo">MirroringInfo
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MirroringInfoSpec">MirroringInfoSpec</a>)
</p>
<div>
<p>MirroringInfo is the mirroring info of a given pool/radosnamespace</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>mode</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mode is the mirroring mode</p>
</td>
</tr>
<tr>
<td>
<code>site_name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SiteName is the current site name</p>
</td>
</tr>
<tr>
<td>
<code>peers</code><br/>
<em>
<a href="#ceph.rook.io/v1.PeersSpec">
[]PeersSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Peers are the list of peer sites connected to that cluster</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringInfoSpec">MirroringInfoSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">CephBlockPoolRadosNamespaceStatus</a>, <a href="#ceph.rook.io/v1.CephBlockPoolStatus">CephBlockPoolStatus</a>)
</p>
<div>
<p>MirroringInfoSpec is the status of the pool/radosnamespace mirroring</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>MirroringInfo</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringInfo">
MirroringInfo
</a>
</em>
</td>
<td>
<p>
(Members of <code>MirroringInfo</code> are embedded into this type.)
</p>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>lastChecked</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>lastChanged</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>details</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringPeerSpec">MirroringPeerSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FSMirroringSpec">FSMirroringSpec</a>, <a href="#ceph.rook.io/v1.MirroringSpec">MirroringSpec</a>, <a href="#ceph.rook.io/v1.RBDMirroringSpec">RBDMirroringSpec</a>)
</p>
<div>
<p>MirroringPeerSpec represents the specification of a mirror peer</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>secretNames</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecretNames represents the Kubernetes Secret names to add rbd-mirror or cephfs-mirror peers</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringSpec">MirroringSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.PoolSpec">PoolSpec</a>)
</p>
<div>
<p>MirroringSpec represents the setting for a mirrored pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled whether this pool is mirrored or not</p>
</td>
</tr>
<tr>
<td>
<code>mode</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Mode is the mirroring mode: pool, image or init-only.</p>
</td>
</tr>
<tr>
<td>
<code>snapshotSchedules</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotScheduleSpec">
[]SnapshotScheduleSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SnapshotSchedules is the scheduling of snapshot for mirrored images/pools</p>
</td>
</tr>
<tr>
<td>
<code>peers</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringPeerSpec">
MirroringPeerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Peers represents the peers spec</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringStatus">MirroringStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MirroringStatusSpec">MirroringStatusSpec</a>)
</p>
<div>
<p>MirroringStatus is the pool/radosNamespace mirror status</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>summary</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringStatusSummarySpec">
MirroringStatusSummarySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Summary is the mirroring status summary</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringStatusSpec">MirroringStatusSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">CephBlockPoolRadosNamespaceStatus</a>, <a href="#ceph.rook.io/v1.CephBlockPoolStatus">CephBlockPoolStatus</a>)
</p>
<div>
<p>MirroringStatusSpec is the status of the pool/radosNamespace mirroring</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>MirroringStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringStatus">
MirroringStatus
</a>
</em>
</td>
<td>
<p>
(Members of <code>MirroringStatus</code> are embedded into this type.)
</p>
<em>(Optional)</em>
<p>MirroringStatus is the mirroring status of a pool/radosNamespace</p>
</td>
</tr>
<tr>
<td>
<code>lastChecked</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChecked is the last time time the status was checked</p>
</td>
</tr>
<tr>
<td>
<code>lastChanged</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChanged is the last time time the status last changed</p>
</td>
</tr>
<tr>
<td>
<code>details</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Details contains potential status errors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MirroringStatusSummarySpec">MirroringStatusSummarySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MirroringStatus">MirroringStatus</a>)
</p>
<div>
<p>MirroringStatusSummarySpec is the summary output of the command</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>health</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Health is the mirroring health</p>
</td>
</tr>
<tr>
<td>
<code>daemon_health</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>DaemonHealth is the health of the mirroring daemon</p>
</td>
</tr>
<tr>
<td>
<code>image_health</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImageHealth is the health of the mirrored image</p>
</td>
</tr>
<tr>
<td>
<code>states</code><br/>
<em>
<a href="#ceph.rook.io/v1.StatesSpec">
StatesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>States is the various state for all mirrored images</p>
</td>
</tr>
<tr>
<td>
<code>image_states</code><br/>
<em>
<a href="#ceph.rook.io/v1.StatesSpec">
StatesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImageStates is the various state for all mirrored images</p>
</td>
</tr>
<tr>
<td>
<code>group_health</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>GroupHealth is the health of the mirrored image group</p>
</td>
</tr>
<tr>
<td>
<code>group_states</code><br/>
<em>
<a href="#ceph.rook.io/v1.StatesSpec">
StatesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>GroupStates is the various state for all mirrored image groups</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Module">Module
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MgrSpec">MgrSpec</a>)
</p>
<div>
<p>Module represents mgr modules that the user wants to enable or disable</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name is the name of the ceph manager module</p>
</td>
</tr>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled determines whether a module should be enabled or not</p>
</td>
</tr>
<tr>
<td>
<code>settings</code><br/>
<em>
<a href="#ceph.rook.io/v1.ModuleSettings">
ModuleSettings
</a>
</em>
</td>
<td>
<p>Settings to further configure the module</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ModuleSettings">ModuleSettings
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.Module">Module</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>balancerMode</code><br/>
<em>
string
</em>
</td>
<td>
<p>BalancerMode sets the <code>balancer</code> module with different modes like <code>upmap</code>, <code>crush-compact</code> etc</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MonSpec">MonSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>MonSpec represents the specification of the monitor</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Count is the number of Ceph monitors</p>
</td>
</tr>
<tr>
<td>
<code>allowMultiplePerNode</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>AllowMultiplePerNode determines if we can run multiple monitors on the same node (not recommended)</p>
</td>
</tr>
<tr>
<td>
<code>failureDomainLabel</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>zones</code><br/>
<em>
<a href="#ceph.rook.io/v1.MonZoneSpec">
[]MonZoneSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Zones are specified when we want to provide zonal awareness to mons</p>
</td>
</tr>
<tr>
<td>
<code>stretchCluster</code><br/>
<em>
<a href="#ceph.rook.io/v1.StretchClusterSpec">
StretchClusterSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>StretchCluster is the stretch cluster specification</p>
</td>
</tr>
<tr>
<td>
<code>volumeClaimTemplate</code><br/>
<em>
<a href="#ceph.rook.io/v1.VolumeClaimTemplate">
VolumeClaimTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>VolumeClaimTemplate is the PVC definition</p>
</td>
</tr>
<tr>
<td>
<code>externalMonIDs</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ExternalMonIDs - optional list of monitor IDs which are deployed externally and not managed by Rook.
If set, Rook will not remove mons with given IDs from quorum.
This parameter is used only for local Rook cluster running in normal mode
and will be ignored if external or stretched mode is used.
leading</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MonZoneSpec">MonZoneSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MonSpec">MonSpec</a>, <a href="#ceph.rook.io/v1.StretchClusterSpec">StretchClusterSpec</a>)
</p>
<div>
<p>MonZoneSpec represents the specification of a zone in a Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Name is the name of the zone</p>
</td>
</tr>
<tr>
<td>
<code>arbiter</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Arbiter determines if the zone contains the arbiter used for stretch cluster mode</p>
</td>
</tr>
<tr>
<td>
<code>volumeClaimTemplate</code><br/>
<em>
<a href="#ceph.rook.io/v1.VolumeClaimTemplate">
VolumeClaimTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>VolumeClaimTemplate is the PVC template</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MonitoringSpec">MonitoringSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>MonitoringSpec represents the settings for Prometheus based Ceph monitoring</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enabled determines whether to create the prometheus rules for the ceph cluster. If true, the prometheus
types must exist or the creation will fail. Default is false.</p>
</td>
</tr>
<tr>
<td>
<code>metricsDisabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to disable the metrics reported by Ceph. If false, the prometheus mgr module and Ceph exporter are enabled.
If true, the prometheus mgr module and Ceph exporter are both disabled. Default is false.</p>
</td>
</tr>
<tr>
<td>
<code>externalMgrEndpoints</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#endpointaddress-v1-core">
[]Kubernetes core/v1.EndpointAddress
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ExternalMgrEndpoints points to an existing Ceph prometheus exporter endpoint</p>
</td>
</tr>
<tr>
<td>
<code>externalMgrPrometheusPort</code><br/>
<em>
uint16
</em>
</td>
<td>
<em>(Optional)</em>
<p>ExternalMgrPrometheusPort Prometheus exporter port</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Port is the prometheus server port</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Duration">
Kubernetes meta/v1.Duration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Interval determines prometheus scrape interval</p>
</td>
</tr>
<tr>
<td>
<code>exporter</code><br/>
<em>
<a href="#ceph.rook.io/v1.CephExporterSpec">
CephExporterSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Ceph exporter configuration</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.MultiClusterServiceSpec">MultiClusterServiceSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NetworkSpec">NetworkSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enable multiClusterService to export the mon and OSD services to peer cluster.
Ensure that peer clusters are connected using an MCS API compatible application,
like Globalnet Submariner.</p>
</td>
</tr>
<tr>
<td>
<code>clusterID</code><br/>
<em>
string
</em>
</td>
<td>
<p>ClusterID uniquely identifies a cluster. It is used as a prefix to nslookup exported
services. For example: <clusterid>.<svc>.<ns>.svc.clusterset.local</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NFSGaneshaSpec">NFSGaneshaSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephNFS">CephNFS</a>)
</p>
<div>
<p>NFSGaneshaSpec represents the spec of an nfs ganesha server</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>rados</code><br/>
<em>
<a href="#ceph.rook.io/v1.GaneshaRADOSSpec">
GaneshaRADOSSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>RADOS is the Ganesha RADOS specification</p>
</td>
</tr>
<tr>
<td>
<code>server</code><br/>
<em>
<a href="#ceph.rook.io/v1.GaneshaServerSpec">
GaneshaServerSpec
</a>
</em>
</td>
<td>
<p>Server is the Ganesha Server specification</p>
</td>
</tr>
<tr>
<td>
<code>security</code><br/>
<em>
<a href="#ceph.rook.io/v1.NFSSecuritySpec">
NFSSecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Security allows specifying security configurations for the NFS cluster</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NFSSecuritySpec">NFSSecuritySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NFSGaneshaSpec">NFSGaneshaSpec</a>)
</p>
<div>
<p>NFSSecuritySpec represents security configurations for an NFS server pod</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>sssd</code><br/>
<em>
<a href="#ceph.rook.io/v1.SSSDSpec">
SSSDSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SSSD enables integration with System Security Services Daemon (SSSD). SSSD can be used to
provide user ID mapping from a number of sources. See <a href="https://sssd.io">https://sssd.io</a> for more information
about the SSSD project.</p>
</td>
</tr>
<tr>
<td>
<code>kerberos</code><br/>
<em>
<a href="#ceph.rook.io/v1.KerberosSpec">
KerberosSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Kerberos configures NFS-Ganesha to secure NFS client connections with Kerberos.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NamedBlockPoolSpec">NamedBlockPoolSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPool">CephBlockPool</a>)
</p>
<div>
<p>NamedBlockPoolSpec allows a block pool to be created with a non-default name.
This is more specific than the NamedPoolSpec so we get schema validation on the
allowed pool names that can be specified.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The desired name of the pool if different from the CephBlockPool CR name.</p>
</td>
</tr>
<tr>
<td>
<code>PoolSpec</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PoolSpec</code> are embedded into this type.)
</p>
<p>The core pool configuration</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NamedPoolSpec">NamedPoolSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemSpec">FilesystemSpec</a>)
</p>
<div>
<p>NamedPoolSpec represents the named ceph pool spec</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name of the pool</p>
</td>
</tr>
<tr>
<td>
<code>PoolSpec</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<p>
(Members of <code>PoolSpec</code> are embedded into this type.)
</p>
<p>PoolSpec represents the spec of ceph pool</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NetworkProviderType">NetworkProviderType
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NetworkSpec">NetworkSpec</a>)
</p>
<div>
<p>NetworkProviderType defines valid network providers for Rook.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;host&#34;</p></td>
<td></td>
</tr><tr><td><p>&#34;multus&#34;</p></td>
<td></td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.NetworkSpec">NetworkSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>NetworkSpec for Ceph includes backward compatibility code</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>provider</code><br/>
<em>
<a href="#ceph.rook.io/v1.NetworkProviderType">
NetworkProviderType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Provider is what provides network connectivity to the cluster e.g. &ldquo;host&rdquo; or &ldquo;multus&rdquo;.
If the Provider is updated from being empty to &ldquo;host&rdquo; on a running cluster, then the operator will automatically fail over all the mons to apply the &ldquo;host&rdquo; network settings.</p>
</td>
</tr>
<tr>
<td>
<code>selectors</code><br/>
<em>
map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.CephNetworkType]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Selectors define NetworkAttachmentDefinitions to be used for Ceph public and/or cluster
networks when the &ldquo;multus&rdquo; network provider is used. This config section is not used for
other network providers.</p>
<p>Valid keys are &ldquo;public&rdquo; and &ldquo;cluster&rdquo;. Refer to Ceph networking documentation for more:
<a href="https://docs.ceph.com/en/latest/rados/configuration/network-config-ref/">https://docs.ceph.com/en/latest/rados/configuration/network-config-ref/</a></p>
<p>Refer to Multus network annotation documentation for help selecting values:
<a href="https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md#run-pod-with-network-annotation">https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md#run-pod-with-network-annotation</a></p>
<p>Rook will make a best-effort attempt to automatically detect CIDR address ranges for given
network attachment definitions. Rook&rsquo;s methods are robust but may be imprecise for
sufficiently complicated networks. Rook&rsquo;s auto-detection process obtains a new IP address
lease for each CephCluster reconcile. If Rook fails to detect, incorrectly detects, only
partially detects, or if underlying networks do not support reusing old IP addresses, it is
best to use the &lsquo;addressRanges&rsquo; config section to specify CIDR ranges for the Ceph cluster.</p>
<p>As a contrived example, one can use a theoretical Kubernetes-wide network for Ceph client
traffic and a theoretical Rook-only network for Ceph replication traffic as shown:
selectors:
public: &ldquo;default/cluster-fast-net&rdquo;
cluster: &ldquo;rook-ceph/ceph-backend-net&rdquo;</p>
</td>
</tr>
<tr>
<td>
<code>addressRanges</code><br/>
<em>
<a href="#ceph.rook.io/v1.AddressRangesSpec">
AddressRangesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>AddressRanges specify a list of CIDRs that Rook will apply to Ceph&rsquo;s &lsquo;public_network&rsquo; and/or
&lsquo;cluster_network&rsquo; configurations. This config section may be used for the &ldquo;host&rdquo; or &ldquo;multus&rdquo;
network providers.</p>
</td>
</tr>
<tr>
<td>
<code>connections</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConnectionsSpec">
ConnectionsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Settings for network connections such as compression and encryption across the
wire.</p>
</td>
</tr>
<tr>
<td>
<code>hostNetwork</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>HostNetwork to enable host network.
If host networking is enabled or disabled on a running cluster, then the operator will automatically fail over all the mons to
apply the new network settings.</p>
</td>
</tr>
<tr>
<td>
<code>ipFamily</code><br/>
<em>
<a href="#ceph.rook.io/v1.IPFamilyType">
IPFamilyType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>IPFamily is the single stack IPv6 or IPv4 protocol</p>
</td>
</tr>
<tr>
<td>
<code>dualStack</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>DualStack determines whether Ceph daemons should listen on both IPv4 and IPv6</p>
</td>
</tr>
<tr>
<td>
<code>multiClusterService</code><br/>
<em>
<a href="#ceph.rook.io/v1.MultiClusterServiceSpec">
MultiClusterServiceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enable multiClusterService to export the Services between peer clusters</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Node">Node
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.StorageScopeSpec">StorageScopeSpec</a>)
</p>
<div>
<p>Node is a storage nodes</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>config</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>Selection</code><br/>
<em>
<a href="#ceph.rook.io/v1.Selection">
Selection
</a>
</em>
</td>
<td>
<p>
(Members of <code>Selection</code> are embedded into this type.)
</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NodesByName">NodesByName
(<code>[]github.com/rook/rook/pkg/apis/ceph.rook.io/v1.Node</code> alias)</h3>
<div>
<p>NodesByName implements an interface to sort nodes by name</p>
</div>
<h3 id="ceph.rook.io/v1.NotificationFilterRule">NotificationFilterRule
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NotificationFilterSpec">NotificationFilterSpec</a>)
</p>
<div>
<p>NotificationFilterRule represent a single rule in the Notification Filter spec</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name of the metadata or tag</p>
</td>
</tr>
<tr>
<td>
<code>value</code><br/>
<em>
string
</em>
</td>
<td>
<p>Value to filter on</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NotificationFilterSpec">NotificationFilterSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.BucketNotificationSpec">BucketNotificationSpec</a>)
</p>
<div>
<p>NotificationFilterSpec represent the spec of a Bucket Notification filter</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>keyFilters</code><br/>
<em>
<a href="#ceph.rook.io/v1.NotificationKeyFilterRule">
[]NotificationKeyFilterRule
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Filters based on the object&rsquo;s key</p>
</td>
</tr>
<tr>
<td>
<code>metadataFilters</code><br/>
<em>
<a href="#ceph.rook.io/v1.NotificationFilterRule">
[]NotificationFilterRule
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Filters based on the object&rsquo;s metadata</p>
</td>
</tr>
<tr>
<td>
<code>tagFilters</code><br/>
<em>
<a href="#ceph.rook.io/v1.NotificationFilterRule">
[]NotificationFilterRule
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Filters based on the object&rsquo;s tags</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.NotificationKeyFilterRule">NotificationKeyFilterRule
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NotificationFilterSpec">NotificationFilterSpec</a>)
</p>
<div>
<p>NotificationKeyFilterRule represent a single key rule in the Notification Filter spec</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name of the filter - prefix/suffix/regex</p>
</td>
</tr>
<tr>
<td>
<code>value</code><br/>
<em>
string
</em>
</td>
<td>
<p>Value to filter on</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.OSDStatus">OSDStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephStorage">CephStorage</a>)
</p>
<div>
<p>OSDStatus represents OSD status of the ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>storeType</code><br/>
<em>
map[string]int
</em>
</td>
<td>
<p>StoreType is a mapping between the OSD backend stores and number of OSDs using these stores</p>
</td>
</tr>
<tr>
<td>
<code>migrationStatus</code><br/>
<em>
<a href="#ceph.rook.io/v1.MigrationStatus">
MigrationStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.OSDStore">OSDStore
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.StorageScopeSpec">StorageScopeSpec</a>)
</p>
<div>
<p>OSDStore is the backend storage type used for creating the OSDs</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Type of backend storage to be used while creating OSDs. If empty, then bluestore will be used</p>
</td>
</tr>
<tr>
<td>
<code>updateStore</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>UpdateStore updates the backend store for existing OSDs. It destroys each OSD one at a time, cleans up the backing disk
and prepares same OSD on that disk</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectEndpointSpec">ObjectEndpointSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreHostingSpec">ObjectStoreHostingSpec</a>)
</p>
<div>
<p>ObjectEndpointSpec represents an object store endpoint</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>dnsName</code><br/>
<em>
string
</em>
</td>
<td>
<p>DnsName is the DNS name (in RFC-1123 format) of the endpoint.
If the DNS name corresponds to an endpoint with DNS wildcard support, do not include the
wildcard itself in the list of hostnames.
E.g., use &ldquo;mystore.example.com&rdquo; instead of &ldquo;*.mystore.example.com&rdquo;.</p>
</td>
</tr>
<tr>
<td>
<code>port</code><br/>
<em>
int32
</em>
</td>
<td>
<p>Port is the port on which S3 connections can be made for this endpoint.</p>
</td>
</tr>
<tr>
<td>
<code>useTls</code><br/>
<em>
bool
</em>
</td>
<td>
<p>UseTls defines whether the endpoint uses TLS (HTTPS) or not (HTTP).</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectEndpoints">ObjectEndpoints
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreStatus">ObjectStoreStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>insecure</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>secure</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectHealthCheckSpec">ObjectHealthCheckSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>ObjectHealthCheckSpec represents the health check of an object store</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>readinessProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProbeSpec">
ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>startupProbe</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProbeSpec">
ProbeSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectRealmSpec">ObjectRealmSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectRealm">CephObjectRealm</a>)
</p>
<div>
<p>ObjectRealmSpec represent the spec of an ObjectRealm</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>pull</code><br/>
<em>
<a href="#ceph.rook.io/v1.PullSpec">
PullSpec
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectSharedPoolsSpec">ObjectSharedPoolsSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>, <a href="#ceph.rook.io/v1.ObjectZoneSpec">ObjectZoneSpec</a>)
</p>
<div>
<p>ObjectSharedPoolsSpec represents object store pool info when configuring RADOS namespaces in existing pools.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The metadata pool used for creating RADOS namespaces in the object store</p>
</td>
</tr>
<tr>
<td>
<code>dataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool used for creating RADOS namespaces in the object store</p>
</td>
</tr>
<tr>
<td>
<code>preserveRadosNamespaceDataOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether the RADOS namespaces should be preserved on deletion of the object store</p>
</td>
</tr>
<tr>
<td>
<code>poolPlacements</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolPlacementSpec">
[]PoolPlacementSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PoolPlacements control which Pools are associated with a particular RGW bucket.
Once PoolPlacements are defined, RGW client will be able to associate pool
with ObjectStore bucket by providing &ldquo;<LocationConstraint>&rdquo; during s3 bucket creation
or &ldquo;X-Storage-Policy&rdquo; header during swift container creation.
See: <a href="https://docs.ceph.com/en/latest/radosgw/placement/#placement-targets">https://docs.ceph.com/en/latest/radosgw/placement/#placement-targets</a>
PoolPlacement with name: &ldquo;default&rdquo; will be used as a default pool if no option
is provided during bucket creation.
If default placement is not provided, spec.sharedPools.dataPoolName and spec.sharedPools.MetadataPoolName will be used as default pools.
If spec.sharedPools are also empty, then RGW pools (spec.dataPool and spec.metadataPool) will be used as defaults.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectStoreAPI">ObjectStoreAPI
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ProtocolSpec">ProtocolSpec</a>)
</p>
<div>
</div>
<h3 id="ceph.rook.io/v1.ObjectStoreHostingSpec">ObjectStoreHostingSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>ObjectStoreHostingSpec represents the hosting settings for the object store</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>advertiseEndpoint</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectEndpointSpec">
ObjectEndpointSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>AdvertiseEndpoint is the default endpoint Rook will return for resources dependent on this
object store. This endpoint will be returned to CephObjectStoreUsers, Object Bucket Claims,
and COSI Buckets/Accesses.
By default, Rook returns the endpoint for the object store&rsquo;s Kubernetes service using HTTPS
with <code>gateway.securePort</code> if it is defined (otherwise, HTTP with <code>gateway.port</code>).</p>
</td>
</tr>
<tr>
<td>
<code>dnsNames</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>A list of DNS host names on which object store gateways will accept client S3 connections.
When specified, object store gateways will reject client S3 connections to hostnames that are
not present in this list, so include all endpoints.
The object store&rsquo;s advertiseEndpoint and Kubernetes service endpoint, plus CephObjectZone
<code>customEndpoints</code> are automatically added to the list but may be set here again if desired.
Each DNS name must be valid according RFC-1123.
If the DNS name corresponds to an endpoint with DNS wildcard support, do not include the
wildcard itself in the list of hostnames.
E.g., use &ldquo;mystore.example.com&rdquo; instead of &ldquo;*.mystore.example.com&rdquo;.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectStoreSecuritySpec">ObjectStoreSecuritySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>ObjectStoreSecuritySpec is spec to define security features like encryption</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>SecuritySpec</code><br/>
<em>
<a href="#ceph.rook.io/v1.SecuritySpec">
SecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>s3</code><br/>
<em>
<a href="#ceph.rook.io/v1.KeyManagementServiceSpec">
KeyManagementServiceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The settings for supporting AWS-SSE:S3 with RGW</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectStore">CephObjectStore</a>)
</p>
<div>
<p>ObjectStoreSpec represent the spec of a pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The metadata pool settings</p>
</td>
</tr>
<tr>
<td>
<code>dataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool settings</p>
</td>
</tr>
<tr>
<td>
<code>sharedPools</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectSharedPoolsSpec">
ObjectSharedPoolsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The pool information when configuring RADOS namespaces in existing pools.</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolsOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pools on object store deletion</p>
</td>
</tr>
<tr>
<td>
<code>gateway</code><br/>
<em>
<a href="#ceph.rook.io/v1.GatewaySpec">
GatewaySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The rgw pod info</p>
</td>
</tr>
<tr>
<td>
<code>protocols</code><br/>
<em>
<a href="#ceph.rook.io/v1.ProtocolSpec">
ProtocolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The protocol specification</p>
</td>
</tr>
<tr>
<td>
<code>auth</code><br/>
<em>
<a href="#ceph.rook.io/v1.AuthSpec">
AuthSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The authentication configuration</p>
</td>
</tr>
<tr>
<td>
<code>zone</code><br/>
<em>
<a href="#ceph.rook.io/v1.ZoneSpec">
ZoneSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The multisite info</p>
</td>
</tr>
<tr>
<td>
<code>healthCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectHealthCheckSpec">
ObjectHealthCheckSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The RGW health probes</p>
</td>
</tr>
<tr>
<td>
<code>security</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreSecuritySpec">
ObjectStoreSecuritySpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Security represents security settings</p>
</td>
</tr>
<tr>
<td>
<code>allowUsersInNamespaces</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The list of allowed namespaces in addition to the object store namespace
where ceph object store users may be created. Specify &ldquo;*&rdquo; to allow all
namespaces, otherwise list individual namespaces that are to be allowed.
This is useful for applications that need object store credentials
to be created in their own namespace, where neither OBCs nor COSI
is being used to create buckets. The default is empty.</p>
</td>
</tr>
<tr>
<td>
<code>hosting</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreHostingSpec">
ObjectStoreHostingSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Hosting settings for the object store.
A common use case for hosting configuration is to inform Rook of endpoints that support DNS
wildcards, which in turn allows virtual host-style bucket addressing.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectStoreStatus">ObjectStoreStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectStore">CephObjectStore</a>)
</p>
<div>
<p>ObjectStoreStatus represents the status of a Ceph Object Store resource</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConditionType">
ConditionType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>endpoints</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectEndpoints">
ObjectEndpoints
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>cephx</code><br/>
<em>
<a href="#ceph.rook.io/v1.LocalCephxStatus">
LocalCephxStatus
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#ceph.rook.io/v1.Condition">
[]Condition
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectStoreUserSpec">ObjectStoreUserSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectStoreUser">CephObjectStoreUser</a>)
</p>
<div>
<p>ObjectStoreUserSpec represent the spec of an Objectstoreuser</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>store</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The store the user will be created in</p>
</td>
</tr>
<tr>
<td>
<code>displayName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The display name for the ceph users</p>
</td>
</tr>
<tr>
<td>
<code>capabilities</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectUserCapSpec">
ObjectUserCapSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>quotas</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectUserQuotaSpec">
ObjectUserQuotaSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>keys</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectUserKey">
[]ObjectUserKey
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Allows specifying credentials for the user. If not provided, the operator
will generate them.</p>
</td>
</tr>
<tr>
<td>
<code>clusterNamespace</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The namespace where the parent CephCluster and CephObjectStore are found</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectStoreUserStatus">ObjectStoreUserStatus
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectStoreUser">CephObjectStoreUser</a>)
</p>
<div>
<p>ObjectStoreUserStatus represents the status Ceph Object Store Gateway User</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
<tr>
<td>
<code>keys</code><br/>
<em>
<a href="#ceph.rook.io/v1.SecretReference">
[]SecretReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectUserCapSpec">ObjectUserCapSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreUserSpec">ObjectStoreUserSpec</a>)
</p>
<div>
<p>Additional admin-level capabilities for the Ceph object store user</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>user</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store users. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>users</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store users. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>bucket</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store buckets. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>buckets</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store buckets. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>metadata</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store metadata. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>usage</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store usage. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>zone</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write Ceph object store zones. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>roles</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write roles for user. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>info</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Admin capabilities to read/write information about the user. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>amz-cache</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to send request to RGW Cache API header. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/rgw-cache/#cache-api">https://docs.ceph.com/en/latest/radosgw/rgw-cache/#cache-api</a></p>
</td>
</tr>
<tr>
<td>
<code>bilog</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to change bucket index logging. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>mdlog</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to change metadata logging. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>datalog</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to change data logging. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>user-policy</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to change user policies. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>oidc-provider</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to change oidc provider. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
<tr>
<td>
<code>ratelimit</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Add capabilities for user to set rate limiter for user and bucket. Documented in <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities">https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectUserKey">ObjectUserKey
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreUserSpec">ObjectStoreUserSpec</a>)
</p>
<div>
<p>ObjectUserKey defines a set of rgw user access credentials to be retrieved
from secret resources.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>accessKeyRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>Secret key selector for the access_key (commonly referred to as AWS_ACCESS_KEY_ID).</p>
</td>
</tr>
<tr>
<td>
<code>secretKeyRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretkeyselector-v1-core">
Kubernetes core/v1.SecretKeySelector
</a>
</em>
</td>
<td>
<p>Secret key selector for the secret_key (commonly referred to as AWS_SECRET_ACCESS_KEY).</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectUserQuotaSpec">ObjectUserQuotaSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreUserSpec">ObjectStoreUserSpec</a>)
</p>
<div>
<p>ObjectUserQuotaSpec can be used to set quotas for the object store user to limit their usage. See the <a href="https://docs.ceph.com/en/latest/radosgw/admin/?#quota-management">Ceph docs</a> for more</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>maxBuckets</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Maximum bucket limit for the ceph user</p>
</td>
</tr>
<tr>
<td>
<code>maxSize</code><br/>
<em>
k8s.io/apimachinery/pkg/api/resource.Quantity
</em>
</td>
<td>
<em>(Optional)</em>
<p>Maximum size limit of all objects across all the user&rsquo;s buckets
See <a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity">https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity</a> for more info.</p>
</td>
</tr>
<tr>
<td>
<code>maxObjects</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>Maximum number of objects across all the user&rsquo;s buckets</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectZoneGroupSpec">ObjectZoneGroupSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectZoneGroup">CephObjectZoneGroup</a>)
</p>
<div>
<p>ObjectZoneGroupSpec represent the spec of an ObjectZoneGroup</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>realm</code><br/>
<em>
string
</em>
</td>
<td>
<p>The display name for the ceph users</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ObjectZoneSpec">ObjectZoneSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephObjectZone">CephObjectZone</a>)
</p>
<div>
<p>ObjectZoneSpec represent the spec of an ObjectZone</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>zoneGroup</code><br/>
<em>
string
</em>
</td>
<td>
<p>The display name for the ceph users</p>
</td>
</tr>
<tr>
<td>
<code>metadataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The metadata pool settings</p>
</td>
</tr>
<tr>
<td>
<code>dataPool</code><br/>
<em>
<a href="#ceph.rook.io/v1.PoolSpec">
PoolSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool settings</p>
</td>
</tr>
<tr>
<td>
<code>sharedPools</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectSharedPoolsSpec">
ObjectSharedPoolsSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The pool information when configuring RADOS namespaces in existing pools.</p>
</td>
</tr>
<tr>
<td>
<code>customEndpoints</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If this zone cannot be accessed from other peer Ceph clusters via the ClusterIP Service
endpoint created by Rook, you must set this to the externally reachable endpoint(s). You may
include the port in the definition. For example: &ldquo;<a href="https://my-object-store.my-domain.net:443&quot;">https://my-object-store.my-domain.net:443&rdquo;</a>.
In many cases, you should set this to the endpoint of the ingress resource that makes the
CephObjectStore associated with this CephObjectStoreZone reachable to peer clusters.
The list can have one or more endpoints pointing to different RGW servers in the zone.</p>
<p>If a CephObjectStore endpoint is omitted from this list, that object store&rsquo;s gateways will
not receive multisite replication data
(see CephObjectStore.spec.gateway.disableMultisiteSyncTraffic).</p>
</td>
</tr>
<tr>
<td>
<code>preservePoolsOnDelete</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Preserve pools on object zone deletion</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.OpsLogSidecar">OpsLogSidecar
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>)
</p>
<div>
<p>RGWLoggingSpec is intended to extend the s3/swift logging for client operations</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources represents the way to specify resource requirements for the ops-log sidecar</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PeerRemoteSpec">PeerRemoteSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemMirrorInfoPeerSpec">FilesystemMirrorInfoPeerSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>client_name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ClientName is cephx name</p>
</td>
</tr>
<tr>
<td>
<code>cluster_name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ClusterName is the name of the cluster</p>
</td>
</tr>
<tr>
<td>
<code>fs_name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>FsName is the filesystem name</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PeerStatSpec">PeerStatSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FilesystemMirrorInfoPeerSpec">FilesystemMirrorInfoPeerSpec</a>)
</p>
<div>
<p>PeerStatSpec are the mirror stat with a given peer</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>failure_count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>FailureCount is the number of mirroring failure</p>
</td>
</tr>
<tr>
<td>
<code>recovery_count</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>RecoveryCount is the number of recovery attempted after failures</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PeersSpec">PeersSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MirroringInfo">MirroringInfo</a>)
</p>
<div>
<p>PeersSpec contains peer details</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>uuid</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>UUID is the peer UUID</p>
</td>
</tr>
<tr>
<td>
<code>direction</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Direction is the peer mirroring direction</p>
</td>
</tr>
<tr>
<td>
<code>site_name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SiteName is the current site name</p>
</td>
</tr>
<tr>
<td>
<code>mirror_uuid</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>MirrorUUID is the mirror UUID</p>
</td>
</tr>
<tr>
<td>
<code>client_name</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ClientName is the CephX user used to connect to the peer</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Placement">Placement
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephCOSIDriverSpec">CephCOSIDriverSpec</a>, <a href="#ceph.rook.io/v1.FilesystemMirroringSpec">FilesystemMirroringSpec</a>, <a href="#ceph.rook.io/v1.GaneshaServerSpec">GaneshaServerSpec</a>, <a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>, <a href="#ceph.rook.io/v1.MetadataServerSpec">MetadataServerSpec</a>, <a href="#ceph.rook.io/v1.RBDMirroringSpec">RBDMirroringSpec</a>, <a href="#ceph.rook.io/v1.StorageClassDeviceSet">StorageClassDeviceSet</a>)
</p>
<div>
<p>Placement is the placement for an object</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>nodeAffinity</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#nodeaffinity-v1-core">
Kubernetes core/v1.NodeAffinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>NodeAffinity is a group of node affinity scheduling rules</p>
</td>
</tr>
<tr>
<td>
<code>podAffinity</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#podaffinity-v1-core">
Kubernetes core/v1.PodAffinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PodAffinity is a group of inter pod affinity scheduling rules</p>
</td>
</tr>
<tr>
<td>
<code>podAntiAffinity</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#podantiaffinity-v1-core">
Kubernetes core/v1.PodAntiAffinity
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PodAntiAffinity is a group of inter pod anti affinity scheduling rules</p>
</td>
</tr>
<tr>
<td>
<code>tolerations</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#toleration-v1-core">
[]Kubernetes core/v1.Toleration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The pod this Toleration is attached to tolerates any taint that matches
the triple <key,value,effect> using the matching operator <operator></p>
</td>
</tr>
<tr>
<td>
<code>topologySpreadConstraints</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#topologyspreadconstraint-v1-core">
[]Kubernetes core/v1.TopologySpreadConstraint
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>TopologySpreadConstraints specifies how to spread matching pods among the given topology</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PlacementSpec">PlacementSpec
(<code>map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.KeyType]github.com/rook/rook/pkg/apis/ceph.rook.io/v1.Placement</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>PlacementSpec is the placement for core ceph daemons part of the CephCluster CRD</p>
</div>
<h3 id="ceph.rook.io/v1.PlacementStorageClassSpec">PlacementStorageClassSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.PoolPlacementSpec">PoolPlacementSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name is the StorageClass name. Ceph allows arbitrary name for StorageClasses,
however most clients/libs insist on AWS names so it is recommended to use
one of the valid x-amz-storage-class values for better compatibility:
REDUCED_REDUNDANCY | STANDARD_IA | ONEZONE_IA | INTELLIGENT_TIERING | GLACIER | DEEP_ARCHIVE | OUTPOSTS | GLACIER_IR | SNOW | EXPRESS_ONEZONE
See AWS docs: <a href="https://aws.amazon.com/de/s3/storage-classes/">https://aws.amazon.com/de/s3/storage-classes/</a></p>
</td>
</tr>
<tr>
<td>
<code>dataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<p>DataPoolName is the data pool used to store ObjectStore objects data.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PoolPlacementSpec">PoolPlacementSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectSharedPoolsSpec">ObjectSharedPoolsSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Pool placement name. Name can be arbitrary. Placement with name &ldquo;default&rdquo; will be used as default.</p>
</td>
</tr>
<tr>
<td>
<code>default</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Sets given placement as default. Only one placement in the list can be marked as default.
Default is false.</p>
</td>
</tr>
<tr>
<td>
<code>metadataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<p>The metadata pool used to store ObjectStore bucket index.</p>
</td>
</tr>
<tr>
<td>
<code>dataPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<p>The data pool used to store ObjectStore objects data.</p>
</td>
</tr>
<tr>
<td>
<code>dataNonECPoolName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The data pool used to store ObjectStore data that cannot use erasure coding (ex: multi-part uploads).
If dataPoolName is not erasure coded, then there is no need for dataNonECPoolName.</p>
</td>
</tr>
<tr>
<td>
<code>storageClasses</code><br/>
<em>
<a href="#ceph.rook.io/v1.PlacementStorageClassSpec">
[]PlacementStorageClassSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>StorageClasses can be selected by user to override dataPoolName during object creation.
Each placement has default STANDARD StorageClass pointing to dataPoolName.
This list allows defining additional StorageClasses on top of default STANDARD storage class.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PoolSpec">PoolSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NamedBlockPoolSpec">NamedBlockPoolSpec</a>, <a href="#ceph.rook.io/v1.NamedPoolSpec">NamedPoolSpec</a>, <a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>, <a href="#ceph.rook.io/v1.ObjectZoneSpec">ObjectZoneSpec</a>)
</p>
<div>
<p>PoolSpec represents the spec of ceph pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>failureDomain</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The failure domain: osd/host/(region or zone if available) - technically also any type in the crush map</p>
</td>
</tr>
<tr>
<td>
<code>crushRoot</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The root of the crush hierarchy utilized by the pool</p>
</td>
</tr>
<tr>
<td>
<code>deviceClass</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The device class the OSD should set to for use in the pool</p>
</td>
</tr>
<tr>
<td>
<code>enableCrushUpdates</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Allow rook operator to change the pool CRUSH tunables once the pool is created</p>
</td>
</tr>
<tr>
<td>
<code>compressionMode</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>DEPRECATED: use Parameters instead, e.g., Parameters[&ldquo;compression_mode&rdquo;] = &ldquo;force&rdquo;
The inline compression mode in Bluestore OSD to set to (options are: none, passive, aggressive, force)
Do NOT set a default value for kubebuilder as this will override the Parameters</p>
</td>
</tr>
<tr>
<td>
<code>replicated</code><br/>
<em>
<a href="#ceph.rook.io/v1.ReplicatedSpec">
ReplicatedSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The replication settings</p>
</td>
</tr>
<tr>
<td>
<code>erasureCoded</code><br/>
<em>
<a href="#ceph.rook.io/v1.ErasureCodedSpec">
ErasureCodedSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The erasure code settings</p>
</td>
</tr>
<tr>
<td>
<code>parameters</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Parameters is a list of properties to enable on a given pool</p>
</td>
</tr>
<tr>
<td>
<code>enableRBDStats</code><br/>
<em>
bool
</em>
</td>
<td>
<p>EnableRBDStats is used to enable gathering of statistics for all RBD images in the pool</p>
</td>
</tr>
<tr>
<td>
<code>mirroring</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringSpec">
MirroringSpec
</a>
</em>
</td>
<td>
<p>The mirroring settings</p>
</td>
</tr>
<tr>
<td>
<code>statusCheck</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirrorHealthCheckSpec">
MirrorHealthCheckSpec
</a>
</em>
</td>
<td>
<p>The mirroring statusCheck</p>
</td>
</tr>
<tr>
<td>
<code>quotas</code><br/>
<em>
<a href="#ceph.rook.io/v1.QuotaSpec">
QuotaSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The quota settings</p>
</td>
</tr>
<tr>
<td>
<code>application</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The application name to set on the pool. Only expected to be set for rgw pools.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PriorityClassNamesSpec">PriorityClassNamesSpec
(<code>map[github.com/rook/rook/pkg/apis/ceph.rook.io/v1.KeyType]string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>PriorityClassNamesSpec is a map of priority class names to be assigned to components</p>
</div>
<h3 id="ceph.rook.io/v1.ProbeSpec">ProbeSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.GaneshaServerSpec">GaneshaServerSpec</a>, <a href="#ceph.rook.io/v1.MetadataServerSpec">MetadataServerSpec</a>, <a href="#ceph.rook.io/v1.ObjectHealthCheckSpec">ObjectHealthCheckSpec</a>)
</p>
<div>
<p>ProbeSpec is a wrapper around Probe so it can be enabled or disabled for a Ceph daemon</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>disabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Disabled determines whether probe is disable or not</p>
</td>
</tr>
<tr>
<td>
<code>probe</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#probe-v1-core">
Kubernetes core/v1.Probe
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Probe describes a health check to be performed against a container to determine whether it is
alive or ready to receive traffic.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ProtocolSpec">ProtocolSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>ProtocolSpec represents a Ceph Object Store protocol specification</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enableAPIs</code><br/>
<em>
<a href="#ceph.rook.io/v1.ObjectStoreAPI">
[]ObjectStoreAPI
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Represents RGW &lsquo;rgw_enable_apis&rsquo; config option. See: <a href="https://docs.ceph.com/en/reef/radosgw/config-ref/#confval-rgw_enable_apis">https://docs.ceph.com/en/reef/radosgw/config-ref/#confval-rgw_enable_apis</a>
If no value provided then all APIs will be enabled: s3, s3website, swift, swift_auth, admin, sts, iam, notifications
If enabled APIs are set, all remaining APIs will be disabled.
This option overrides S3.Enabled value.</p>
</td>
</tr>
<tr>
<td>
<code>s3</code><br/>
<em>
<a href="#ceph.rook.io/v1.S3Spec">
S3Spec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The spec for S3</p>
</td>
</tr>
<tr>
<td>
<code>swift</code><br/>
<em>
<a href="#ceph.rook.io/v1.SwiftSpec">
SwiftSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The spec for Swift</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.PullSpec">PullSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectRealmSpec">ObjectRealmSpec</a>)
</p>
<div>
<p>PullSpec represents the pulling specification of a Ceph Object Storage Gateway Realm</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>endpoint</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.QuotaSpec">QuotaSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.PoolSpec">PoolSpec</a>)
</p>
<div>
<p>QuotaSpec represents the spec for quotas in a pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>maxBytes</code><br/>
<em>
uint64
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxBytes represents the quota in bytes
Deprecated in favor of MaxSize</p>
</td>
</tr>
<tr>
<td>
<code>maxSize</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxSize represents the quota in bytes as a string</p>
</td>
</tr>
<tr>
<td>
<code>maxObjects</code><br/>
<em>
uint64
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxObjects represents the quota in objects</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.RBDMirroringSpec">RBDMirroringSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephRBDMirror">CephRBDMirror</a>)
</p>
<div>
<p>RBDMirroringSpec represents the specification of an RBD mirror daemon</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>count</code><br/>
<em>
int
</em>
</td>
<td>
<p>Count represents the number of rbd mirror instance to run</p>
</td>
</tr>
<tr>
<td>
<code>peers</code><br/>
<em>
<a href="#ceph.rook.io/v1.MirroringPeerSpec">
MirroringPeerSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Peers represents the peers spec</p>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The affinity to place the rgw pods (default is to place on any available node)</p>
</td>
</tr>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The annotations-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>labels</code><br/>
<em>
<a href="#ceph.rook.io/v1.Labels">
Labels
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The labels-related configuration to add/set on each Pod related object.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>The resource requirements for the rbd mirror pods</p>
</td>
</tr>
<tr>
<td>
<code>priorityClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>PriorityClassName sets priority class on the rbd mirror pods</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.RGWServiceSpec">RGWServiceSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>)
</p>
<div>
<p>RGWServiceSpec represent the spec for RGW service</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>annotations</code><br/>
<em>
<a href="#ceph.rook.io/v1.Annotations">
Annotations
</a>
</em>
</td>
<td>
<p>The annotations-related configuration to add/set on each rgw service.
nullable
optional</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.RadosNamespaceMirroring">RadosNamespaceMirroring
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceSpec">CephBlockPoolRadosNamespaceSpec</a>)
</p>
<div>
<p>RadosNamespaceMirroring represents the mirroring configuration of CephBlockPoolRadosNamespace</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>remoteNamespace</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>RemoteNamespace is the name of the CephBlockPoolRadosNamespace on the secondary cluster CephBlockPool</p>
</td>
</tr>
<tr>
<td>
<code>mode</code><br/>
<em>
<a href="#ceph.rook.io/v1.RadosNamespaceMirroringMode">
RadosNamespaceMirroringMode
</a>
</em>
</td>
<td>
<p>Mode is the mirroring mode; either pool or image.</p>
</td>
</tr>
<tr>
<td>
<code>snapshotSchedules</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotScheduleSpec">
[]SnapshotScheduleSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SnapshotSchedules is the scheduling of snapshot for mirrored images</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.RadosNamespaceMirroringMode">RadosNamespaceMirroringMode
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.RadosNamespaceMirroring">RadosNamespaceMirroring</a>)
</p>
<div>
<p>RadosNamespaceMirroringMode represents the mode of the RadosNamespace</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;image&#34;</p></td>
<td><p>RadosNamespaceMirroringModeImage represents the image mode</p>
</td>
</tr><tr><td><p>&#34;pool&#34;</p></td>
<td><p>RadosNamespaceMirroringModePool represents the pool mode</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.ReadAffinitySpec">ReadAffinitySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CSIDriverSpec">CSIDriverSpec</a>)
</p>
<div>
<p>ReadAffinitySpec defines the read affinity settings for CSI driver.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enables read affinity for CSI driver.</p>
</td>
</tr>
<tr>
<td>
<code>crushLocationLabels</code><br/>
<em>
[]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>CrushLocationLabels defines which node labels to use
as CRUSH location. This should correspond to the values set in
the CRUSH map.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ReplicatedSpec">ReplicatedSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.PoolSpec">PoolSpec</a>)
</p>
<div>
<p>ReplicatedSpec represents the spec for replication in a pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>size</code><br/>
<em>
uint
</em>
</td>
<td>
<p>Size - Number of copies per object in a replicated storage pool, including the object itself (required for replicated pool type)</p>
</td>
</tr>
<tr>
<td>
<code>targetSizeRatio</code><br/>
<em>
float64
</em>
</td>
<td>
<em>(Optional)</em>
<p>TargetSizeRatio gives a hint (%) to Ceph in terms of expected consumption of the total cluster capacity</p>
</td>
</tr>
<tr>
<td>
<code>requireSafeReplicaSize</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>RequireSafeReplicaSize if false allows you to set replica 1</p>
</td>
</tr>
<tr>
<td>
<code>replicasPerFailureDomain</code><br/>
<em>
uint
</em>
</td>
<td>
<em>(Optional)</em>
<p>ReplicasPerFailureDomain the number of replica in the specified failure domain</p>
</td>
</tr>
<tr>
<td>
<code>subFailureDomain</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SubFailureDomain the name of the sub-failure domain</p>
</td>
</tr>
<tr>
<td>
<code>hybridStorage</code><br/>
<em>
<a href="#ceph.rook.io/v1.HybridStorageSpec">
HybridStorageSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>HybridStorage represents hybrid storage tier settings</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ResourceSpec">ResourceSpec
(<code>map[string]k8s.io/api/core/v1.ResourceRequirements</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
<p>ResourceSpec is a collection of ResourceRequirements that describes the compute resource requirements</p>
</div>
<h3 id="ceph.rook.io/v1.RgwReadAffinity">RgwReadAffinity
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.GatewaySpec">GatewaySpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
string
</em>
</td>
<td>
<p>Type defines the RGW ReadAffinity type
localize: read from the nearest OSD based on crush location of the RGW client
balance: picks a random OSD from the PG&rsquo;s active set
default: read from the primary OSD</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.S3Spec">S3Spec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ProtocolSpec">ProtocolSpec</a>)
</p>
<div>
<p>S3Spec represents Ceph Object Store specification for the S3 API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>enabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Deprecated: use protocol.enableAPIs instead.
Whether to enable S3. This defaults to true (even if protocols.s3 is not present in the CRD). This maintains backwards compatibility – by default S3 is enabled.</p>
</td>
</tr>
<tr>
<td>
<code>authUseKeystone</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to use Keystone for authentication. This option maps directly to the rgw_s3_auth_use_keystone option. Enabling it allows generating S3 credentials via an OpenStack API call, see the docs. If not given, the defaults of the corresponding RGW option apply.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SSSDSidecar">SSSDSidecar
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.SSSDSpec">SSSDSpec</a>)
</p>
<div>
<p>SSSDSidecar represents configuration when SSSD is run in a sidecar.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<p>Image defines the container image that should be used for the SSSD sidecar.</p>
</td>
</tr>
<tr>
<td>
<code>sssdConfigFile</code><br/>
<em>
<a href="#ceph.rook.io/v1.SSSDSidecarConfigFile">
SSSDSidecarConfigFile
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SSSDConfigFile defines where the SSSD configuration should be sourced from. The config file
will be placed into <code>/etc/sssd/sssd.conf</code>. If this is left empty, Rook will not add the file.
This allows you to manage the <code>sssd.conf</code> file yourself however you wish. For example, you
may build it into your custom Ceph container image or use the Vault agent injector to
securely add the file via annotations on the CephNFS spec (passed to the NFS server pods).</p>
</td>
</tr>
<tr>
<td>
<code>additionalFiles</code><br/>
<em>
<a href="#ceph.rook.io/v1.AdditionalVolumeMounts">
AdditionalVolumeMounts
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>AdditionalFiles defines any number of additional files that should be mounted into the SSSD
sidecar with a directory root of <code>/etc/sssd/rook-additional/</code>.
These files may be referenced by the sssd.conf config file.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Resources allow specifying resource requests/limits on the SSSD sidecar container.</p>
</td>
</tr>
<tr>
<td>
<code>debugLevel</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>DebugLevel sets the debug level for SSSD. If unset or set to 0, Rook does nothing. Otherwise,
this may be a value between 1 and 10. See SSSD docs for more info:
<a href="https://sssd.io/troubleshooting/basics.html#sssd-debug-logs">https://sssd.io/troubleshooting/basics.html#sssd-debug-logs</a></p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SSSDSidecarConfigFile">SSSDSidecarConfigFile
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.SSSDSidecar">SSSDSidecar</a>)
</p>
<div>
<p>SSSDSidecarConfigFile represents the source(s) from which the SSSD configuration should come.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>volumeSource</code><br/>
<em>
<a href="#ceph.rook.io/v1.ConfigFileVolumeSource">
ConfigFileVolumeSource
</a>
</em>
</td>
<td>
<p>VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for the
SSSD configuration file like what is normally used to configure Volumes for a Pod. For
example, a ConfigMap, Secret, or HostPath. There are two requirements for the source&rsquo;s
content:
1. The config file must be mountable via <code>subPath: sssd.conf</code>. For example, in a ConfigMap,
the data item must be named <code>sssd.conf</code>, or <code>items</code> must be defined to select the key
and give it path <code>sssd.conf</code>. A HostPath directory must have the <code>sssd.conf</code> file.
2. The volume or config file must have mode 0600.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SSSDSpec">SSSDSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.NFSSecuritySpec">NFSSecuritySpec</a>)
</p>
<div>
<p>SSSDSpec represents configuration for System Security Services Daemon (SSSD).</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>sidecar</code><br/>
<em>
<a href="#ceph.rook.io/v1.SSSDSidecar">
SSSDSidecar
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Sidecar tells Rook to run SSSD in a sidecar alongside the NFS-Ganesha server in each NFS pod.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SanitizeDataSourceProperty">SanitizeDataSourceProperty
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.SanitizeDisksSpec">SanitizeDisksSpec</a>)
</p>
<div>
<p>SanitizeDataSourceProperty represents a sanitizing data source</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;random&#34;</p></td>
<td><p>SanitizeDataSourceRandom uses `shred&rsquo;s default entropy source</p>
</td>
</tr><tr><td><p>&#34;zero&#34;</p></td>
<td><p>SanitizeDataSourceZero uses /dev/zero as sanitize source</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.SanitizeDisksSpec">SanitizeDisksSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CleanupPolicySpec">CleanupPolicySpec</a>)
</p>
<div>
<p>SanitizeDisksSpec represents a disk sanitizing specification</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>method</code><br/>
<em>
<a href="#ceph.rook.io/v1.SanitizeMethodProperty">
SanitizeMethodProperty
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Method is the method we use to sanitize disks</p>
</td>
</tr>
<tr>
<td>
<code>dataSource</code><br/>
<em>
<a href="#ceph.rook.io/v1.SanitizeDataSourceProperty">
SanitizeDataSourceProperty
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DataSource is the data source to use to sanitize the disk with</p>
</td>
</tr>
<tr>
<td>
<code>iteration</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>Iteration is the number of pass to apply the sanitizing</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SanitizeMethodProperty">SanitizeMethodProperty
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.SanitizeDisksSpec">SanitizeDisksSpec</a>)
</p>
<div>
<p>SanitizeMethodProperty represents a disk sanitizing method</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;complete&#34;</p></td>
<td><p>SanitizeMethodComplete will sanitize everything on the disk</p>
</td>
</tr><tr><td><p>&#34;quick&#34;</p></td>
<td><p>SanitizeMethodQuick will sanitize metadata only on the disk</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.SecretReference">SecretReference
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.BucketTopicStatus">BucketTopicStatus</a>, <a href="#ceph.rook.io/v1.ObjectStoreUserStatus">ObjectStoreUserStatus</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>,secretReference</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#secretreference-v1-core">
Kubernetes core/v1.SecretReference
</a>
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>uid</code><br/>
<em>
k8s.io/apimachinery/pkg/types.UID
</em>
</td>
<td>
</td>
</tr>
<tr>
<td>
<code>resourceVersion</code><br/>
<em>
string
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SecuritySpec">SecuritySpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSecuritySpec">ObjectStoreSecuritySpec</a>)
</p>
<div>
<p>SecuritySpec is security spec to include various security items such as kms</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>kms</code><br/>
<em>
<a href="#ceph.rook.io/v1.KeyManagementServiceSpec">
KeyManagementServiceSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeyManagementService is the main Key Management option</p>
</td>
</tr>
<tr>
<td>
<code>keyRotation</code><br/>
<em>
<a href="#ceph.rook.io/v1.KeyRotationSpec">
KeyRotationSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>KeyRotation defines options for Key Rotation.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Selection">Selection
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.Node">Node</a>, <a href="#ceph.rook.io/v1.StorageScopeSpec">StorageScopeSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>useAllDevices</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to consume all the storage devices found on a machine</p>
</td>
</tr>
<tr>
<td>
<code>deviceFilter</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>A regular expression to allow more fine-grained selection of devices on nodes across the cluster</p>
</td>
</tr>
<tr>
<td>
<code>devicePathFilter</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>A regular expression to allow more fine-grained selection of devices with path names</p>
</td>
</tr>
<tr>
<td>
<code>devices</code><br/>
<em>
<a href="#ceph.rook.io/v1.Device">
[]Device
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of devices to use as storage devices</p>
</td>
</tr>
<tr>
<td>
<code>volumeClaimTemplates</code><br/>
<em>
<a href="#ceph.rook.io/v1.VolumeClaimTemplate">
[]VolumeClaimTemplate
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PersistentVolumeClaims to use as storage</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SnapshotSchedule">SnapshotSchedule
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.SnapshotSchedulesSpec">SnapshotSchedulesSpec</a>)
</p>
<div>
<p>SnapshotSchedule is a schedule</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>interval</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Interval is the interval in which snapshots will be taken</p>
</td>
</tr>
<tr>
<td>
<code>start_time</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StartTime is the snapshot starting time</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SnapshotScheduleRetentionSpec">SnapshotScheduleRetentionSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FSMirroringSpec">FSMirroringSpec</a>)
</p>
<div>
<p>SnapshotScheduleRetentionSpec is a retention policy</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>path</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path is the path to snapshot</p>
</td>
</tr>
<tr>
<td>
<code>duration</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Duration represents the retention duration for a snapshot</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SnapshotScheduleSpec">SnapshotScheduleSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.FSMirroringSpec">FSMirroringSpec</a>, <a href="#ceph.rook.io/v1.MirroringSpec">MirroringSpec</a>, <a href="#ceph.rook.io/v1.RadosNamespaceMirroring">RadosNamespaceMirroring</a>)
</p>
<div>
<p>SnapshotScheduleSpec represents the snapshot scheduling settings of a mirrored pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>path</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Path is the path to snapshot, only valid for CephFS</p>
</td>
</tr>
<tr>
<td>
<code>interval</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Interval represent the periodicity of the snapshot.</p>
</td>
</tr>
<tr>
<td>
<code>startTime</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>StartTime indicates when to start the snapshot</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SnapshotScheduleStatusSpec">SnapshotScheduleStatusSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBlockPoolRadosNamespaceStatus">CephBlockPoolRadosNamespaceStatus</a>, <a href="#ceph.rook.io/v1.CephBlockPoolStatus">CephBlockPoolStatus</a>)
</p>
<div>
<p>SnapshotScheduleStatusSpec is the status of the snapshot schedule</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>snapshotSchedules</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotSchedulesSpec">
[]SnapshotSchedulesSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SnapshotSchedules is the list of snapshots scheduled</p>
</td>
</tr>
<tr>
<td>
<code>lastChecked</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChecked is the last time time the status was checked</p>
</td>
</tr>
<tr>
<td>
<code>lastChanged</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>LastChanged is the last time time the status last changed</p>
</td>
</tr>
<tr>
<td>
<code>details</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Details contains potential status errors</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SnapshotSchedulesSpec">SnapshotSchedulesSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.SnapshotScheduleStatusSpec">SnapshotScheduleStatusSpec</a>)
</p>
<div>
<p>SnapshotSchedulesSpec is the list of snapshot scheduled for images in a pool</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>pool</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Pool is the pool name</p>
</td>
</tr>
<tr>
<td>
<code>namespace</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Namespace is the RADOS namespace the image is part of</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Image is the mirrored image</p>
</td>
</tr>
<tr>
<td>
<code>items</code><br/>
<em>
<a href="#ceph.rook.io/v1.SnapshotSchedule">
[]SnapshotSchedule
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Items is the list schedules times for a given snapshot</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.StatesSpec">StatesSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MirroringStatusSummarySpec">MirroringStatusSummarySpec</a>)
</p>
<div>
<p>StatesSpec are rbd images mirroring state</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>starting_replay</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>StartingReplay is when the replay of the mirroring journal starts</p>
</td>
</tr>
<tr>
<td>
<code>replaying</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Replaying is when the replay of the mirroring journal is on-going</p>
</td>
</tr>
<tr>
<td>
<code>syncing</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Syncing is when the image is syncing</p>
</td>
</tr>
<tr>
<td>
<code>stopping_replay</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>StopReplaying is when the replay of the mirroring journal stops</p>
</td>
</tr>
<tr>
<td>
<code>stopped</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Stopped is when the mirroring state is stopped</p>
</td>
</tr>
<tr>
<td>
<code>unknown</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Unknown is when the mirroring state is unknown</p>
</td>
</tr>
<tr>
<td>
<code>error</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>Error is when the mirroring state is errored</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.Status">Status
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.CephBucketNotification">CephBucketNotification</a>, <a href="#ceph.rook.io/v1.CephFilesystemMirror">CephFilesystemMirror</a>, <a href="#ceph.rook.io/v1.CephNFS">CephNFS</a>, <a href="#ceph.rook.io/v1.CephObjectRealm">CephObjectRealm</a>, <a href="#ceph.rook.io/v1.CephObjectZone">CephObjectZone</a>, <a href="#ceph.rook.io/v1.CephObjectZoneGroup">CephObjectZoneGroup</a>, <a href="#ceph.rook.io/v1.CephRBDMirror">CephRBDMirror</a>)
</p>
<div>
<p>Status represents the status of an object</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>phase</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>observedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>ObservedGeneration is the latest generation observed by the controller.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#ceph.rook.io/v1.Condition">
[]Condition
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.StorageClassDeviceSet">StorageClassDeviceSet
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.StorageScopeSpec">StorageScopeSpec</a>)
</p>
<div>
<p>StorageClassDeviceSet is a storage class device set</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>Name is a unique identifier for the set</p>
</td>
</tr>
<tr>
<td>
<code>count</code><br/>
<em>
int
</em>
</td>
<td>
<p>Count is the number of devices in this set</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#resourcerequirements-v1-core">
Kubernetes core/v1.ResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>placement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>preparePlacement</code><br/>
<em>
<a href="#ceph.rook.io/v1.Placement">
Placement
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>config</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Provider-specific device configuration</p>
</td>
</tr>
<tr>
<td>
<code>volumeClaimTemplates</code><br/>
<em>
<a href="#ceph.rook.io/v1.VolumeClaimTemplate">
[]VolumeClaimTemplate
</a>
</em>
</td>
<td>
<p>VolumeClaimTemplates is a list of PVC templates for the underlying storage devices</p>
</td>
</tr>
<tr>
<td>
<code>portable</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Portable represents OSD portability across the hosts</p>
</td>
</tr>
<tr>
<td>
<code>tuneDeviceClass</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>TuneSlowDeviceClass Tune the OSD when running on a slow Device Class</p>
</td>
</tr>
<tr>
<td>
<code>tuneFastDeviceClass</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>TuneFastDeviceClass Tune the OSD when running on a fast Device Class</p>
</td>
</tr>
<tr>
<td>
<code>schedulerName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Scheduler name for OSD pod placement</p>
</td>
</tr>
<tr>
<td>
<code>encrypted</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to encrypt the deviceSet</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.StorageScopeSpec">StorageScopeSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ClusterSpec">ClusterSpec</a>)
</p>
<div>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>nodes</code><br/>
<em>
<a href="#ceph.rook.io/v1.Node">
[]Node
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>useAllNodes</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>scheduleAlways</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to always schedule OSDs on a node even if the node is not currently scheduleable or ready</p>
</td>
</tr>
<tr>
<td>
<code>onlyApplyOSDPlacement</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>config</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>Selection</code><br/>
<em>
<a href="#ceph.rook.io/v1.Selection">
Selection
</a>
</em>
</td>
<td>
<p>
(Members of <code>Selection</code> are embedded into this type.)
</p>
</td>
</tr>
<tr>
<td>
<code>storageClassDeviceSets</code><br/>
<em>
<a href="#ceph.rook.io/v1.StorageClassDeviceSet">
[]StorageClassDeviceSet
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>migration</code><br/>
<em>
<a href="#ceph.rook.io/v1.Migration">
Migration
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Migration handles the OSD migration</p>
</td>
</tr>
<tr>
<td>
<code>store</code><br/>
<em>
<a href="#ceph.rook.io/v1.OSDStore">
OSDStore
</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
<tr>
<td>
<code>flappingRestartIntervalHours</code><br/>
<em>
int
</em>
</td>
<td>
<em>(Optional)</em>
<p>FlappingRestartIntervalHours defines the time for which the OSD pods, that failed with zero exit code, will sleep before restarting.
This is needed for OSD flapping where OSD daemons are marked down more than 5 times in 600 seconds by Ceph.
Preventing the OSD pods to restart immediately in such scenarios will prevent Rook from marking OSD as <code>up</code> and thus
peering of the PGs mapped to the OSD.
User needs to manually restart the OSD pod if they manage to fix the underlying OSD flapping issue before the restart interval.
The sleep will be disabled if this interval is set to 0.</p>
</td>
</tr>
<tr>
<td>
<code>fullRatio</code><br/>
<em>
float64
</em>
</td>
<td>
<em>(Optional)</em>
<p>FullRatio is the ratio at which the cluster is considered full and ceph will stop accepting writes. Default is 0.95.</p>
</td>
</tr>
<tr>
<td>
<code>nearFullRatio</code><br/>
<em>
float64
</em>
</td>
<td>
<em>(Optional)</em>
<p>NearFullRatio is the ratio at which the cluster is considered nearly full and will raise a ceph health warning. Default is 0.85.</p>
</td>
</tr>
<tr>
<td>
<code>backfillFullRatio</code><br/>
<em>
float64
</em>
</td>
<td>
<em>(Optional)</em>
<p>BackfillFullRatio is the ratio at which the cluster is too full for backfill. Backfill will be disabled if above this threshold. Default is 0.90.</p>
</td>
</tr>
<tr>
<td>
<code>allowDeviceClassUpdate</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether to allow updating the device class after the OSD is initially provisioned</p>
</td>
</tr>
<tr>
<td>
<code>allowOsdCrushWeightUpdate</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether Rook will resize the OSD CRUSH weight when the OSD PVC size is increased.
This allows cluster data to be rebalanced to make most effective use of new OSD space.
The default is false since data rebalancing can cause temporary cluster slowdown.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.StoreType">StoreType
(<code>string</code> alias)</h3>
<div>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;bluestore&#34;</p></td>
<td><p>StoreTypeBlueStore is the bluestore backend storage for OSDs</p>
</td>
</tr><tr><td><p>&#34;bluestore-rdr&#34;</p></td>
<td><p>StoreTypeBlueStoreRDR is the bluestore-rdr backed storage for OSDs</p>
</td>
</tr></tbody>
</table>
<h3 id="ceph.rook.io/v1.StretchClusterSpec">StretchClusterSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MonSpec">MonSpec</a>)
</p>
<div>
<p>StretchClusterSpec represents the specification of a stretched Ceph Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>failureDomainLabel</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>FailureDomainLabel the failure domain name (e,g: zone)</p>
</td>
</tr>
<tr>
<td>
<code>subFailureDomain</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>SubFailureDomain is the failure domain within a zone</p>
</td>
</tr>
<tr>
<td>
<code>zones</code><br/>
<em>
<a href="#ceph.rook.io/v1.MonZoneSpec">
[]MonZoneSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Zones is the list of zones</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.SwiftSpec">SwiftSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ProtocolSpec">ProtocolSpec</a>)
</p>
<div>
<p>SwiftSpec represents Ceph Object Store specification for the Swift API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>accountInUrl</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Whether or not the Swift account name should be included in the Swift API URL. If set to false (the default), then the Swift API will listen on a URL formed like <a href="http://host:port/">http://host:port/</a><rgw_swift_url_prefix>/v1. If set to true, the Swift API URL will be <a href="http://host:port/">http://host:port/</a><rgw_swift_url_prefix>/v1/AUTH_<account_name>. You must set this option to true (and update the Keystone service catalog) if you want radosgw to support publicly-readable containers and temporary URLs.</p>
</td>
</tr>
<tr>
<td>
<code>urlPrefix</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The URL prefix for the Swift API, to distinguish it from the S3 API endpoint. The default is swift, which makes the Swift API available at the URL <a href="http://host:port/swift/v1">http://host:port/swift/v1</a> (or <a href="http://host:port/swift/v1/AUTH_%(tenant_id)s">http://host:port/swift/v1/AUTH_%(tenant_id)s</a> if rgw swift account in url is enabled).</p>
</td>
</tr>
<tr>
<td>
<code>versioningEnabled</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>Enables the Object Versioning of OpenStack Object Storage API. This allows clients to put the X-Versions-Location attribute on containers that should be versioned.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.TopicEndpointSpec">TopicEndpointSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.BucketTopicSpec">BucketTopicSpec</a>)
</p>
<div>
<p>TopicEndpointSpec contains exactly one of the endpoint specs of a Bucket Topic</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>http</code><br/>
<em>
<a href="#ceph.rook.io/v1.HTTPEndpointSpec">
HTTPEndpointSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Spec of HTTP endpoint</p>
</td>
</tr>
<tr>
<td>
<code>amqp</code><br/>
<em>
<a href="#ceph.rook.io/v1.AMQPEndpointSpec">
AMQPEndpointSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Spec of AMQP endpoint</p>
</td>
</tr>
<tr>
<td>
<code>kafka</code><br/>
<em>
<a href="#ceph.rook.io/v1.KafkaEndpointSpec">
KafkaEndpointSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Spec of Kafka endpoint</p>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.VolumeClaimTemplate">VolumeClaimTemplate
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.MonSpec">MonSpec</a>, <a href="#ceph.rook.io/v1.MonZoneSpec">MonZoneSpec</a>, <a href="#ceph.rook.io/v1.Selection">Selection</a>, <a href="#ceph.rook.io/v1.StorageClassDeviceSet">StorageClassDeviceSet</a>)
</p>
<div>
<p>VolumeClaimTemplate is a simplified version of K8s corev1&rsquo;s PVC. It has no type meta or status.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>metadata</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Standard object&rsquo;s metadata.
More info: <a href="https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata">https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata</a></p>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#persistentvolumeclaimspec-v1-core">
Kubernetes core/v1.PersistentVolumeClaimSpec
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>spec defines the desired characteristics of a volume requested by a pod author.
More info: <a href="https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims">https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims</a></p>
<br/>
<br/>
<table>
<tr>
<td>
<code>accessModes</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#persistentvolumeaccessmode-v1-core">
[]Kubernetes core/v1.PersistentVolumeAccessMode
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>accessModes contains the desired access modes the volume should have.
More info: <a href="https://kubernetes.io/docs/concepts/storage/persistent-volumes#access-modes-1">https://kubernetes.io/docs/concepts/storage/persistent-volumes#access-modes-1</a></p>
</td>
</tr>
<tr>
<td>
<code>selector</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#labelselector-v1-meta">
Kubernetes meta/v1.LabelSelector
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>selector is a label query over volumes to consider for binding.</p>
</td>
</tr>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#volumeresourcerequirements-v1-core">
Kubernetes core/v1.VolumeResourceRequirements
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>resources represents the minimum resources the volume should have.
If RecoverVolumeExpansionFailure feature is enabled users are allowed to specify resource requirements
that are lower than previous value but must still be higher than capacity recorded in the
status field of the claim.
More info: <a href="https://kubernetes.io/docs/concepts/storage/persistent-volumes#resources">https://kubernetes.io/docs/concepts/storage/persistent-volumes#resources</a></p>
</td>
</tr>
<tr>
<td>
<code>volumeName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>volumeName is the binding reference to the PersistentVolume backing this claim.</p>
</td>
</tr>
<tr>
<td>
<code>storageClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>storageClassName is the name of the StorageClass required by the claim.
More info: <a href="https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1">https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1</a></p>
</td>
</tr>
<tr>
<td>
<code>volumeMode</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#persistentvolumemode-v1-core">
Kubernetes core/v1.PersistentVolumeMode
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>volumeMode defines what type of volume is required by the claim.
Value of Filesystem is implied when not included in claim spec.</p>
</td>
</tr>
<tr>
<td>
<code>dataSource</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#typedlocalobjectreference-v1-core">
Kubernetes core/v1.TypedLocalObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>dataSource field can be used to specify either:
* An existing VolumeSnapshot object (snapshot.storage.k8s.io/VolumeSnapshot)
* An existing PVC (PersistentVolumeClaim)
If the provisioner or an external controller can support the specified data source,
it will create a new volume based on the contents of the specified data source.
When the AnyVolumeDataSource feature gate is enabled, dataSource contents will be copied to dataSourceRef,
and dataSourceRef contents will be copied to dataSource when dataSourceRef.namespace is not specified.
If the namespace is specified, then dataSourceRef will not be copied to dataSource.</p>
</td>
</tr>
<tr>
<td>
<code>dataSourceRef</code><br/>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#typedobjectreference-v1-core">
Kubernetes core/v1.TypedObjectReference
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>dataSourceRef specifies the object from which to populate the volume with data, if a non-empty
volume is desired. This may be any object from a non-empty API group (non
core object) or a PersistentVolumeClaim object.
When this field is specified, volume binding will only succeed if the type of
the specified object matches some installed volume populator or dynamic
provisioner.
This field will replace the functionality of the dataSource field and as such
if both fields are non-empty, they must have the same value. For backwards
compatibility, when namespace isn&rsquo;t specified in dataSourceRef,
both fields (dataSource and dataSourceRef) will be set to the same
value automatically if one of them is empty and the other is non-empty.
When namespace is specified in dataSourceRef,
dataSource isn&rsquo;t set to the same value and must be empty.
There are three important differences between dataSource and dataSourceRef:
* While dataSource only allows two specific types of objects, dataSourceRef
allows any non-core object, as well as PersistentVolumeClaim objects.
* While dataSource ignores disallowed values (dropping them), dataSourceRef
preserves all values, and generates an error if a disallowed value is
specified.
* While dataSource only allows local objects, dataSourceRef allows objects
in any namespaces.
(Beta) Using this field requires the AnyVolumeDataSource feature gate to be enabled.
(Alpha) Using the namespace field of dataSourceRef requires the CrossNamespaceVolumeDataSource feature gate to be enabled.</p>
</td>
</tr>
<tr>
<td>
<code>volumeAttributesClassName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>volumeAttributesClassName may be used to set the VolumeAttributesClass used by this claim.
If specified, the CSI driver will create or update the volume with the attributes defined
in the corresponding VolumeAttributesClass. This has a different purpose than storageClassName,
it can be changed after the claim is created. An empty string value means that no VolumeAttributesClass
will be applied to the claim but it&rsquo;s not allowed to reset this field to empty string once it is set.
If unspecified and the PersistentVolumeClaim is unbound, the default VolumeAttributesClass
will be set by the persistentvolume controller if it exists.
If the resource referred to by volumeAttributesClass does not exist, this PersistentVolumeClaim will be
set to a Pending state, as reflected by the modifyVolumeStatus field, until such as a resource
exists.
More info: <a href="https://kubernetes.io/docs/concepts/storage/volume-attributes-classes/">https://kubernetes.io/docs/concepts/storage/volume-attributes-classes/</a>
(Beta) Using this field requires the VolumeAttributesClass feature gate to be enabled (off by default).</p>
</td>
</tr>
</table>
</td>
</tr>
</tbody>
</table>
<h3 id="ceph.rook.io/v1.ZoneSpec">ZoneSpec
</h3>
<p>
(<em>Appears on:</em><a href="#ceph.rook.io/v1.ObjectStoreSpec">ObjectStoreSpec</a>)
</p>
<div>
<p>ZoneSpec represents a Ceph Object Store Gateway Zone specification</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code><br/>
<em>
string
</em>
</td>
<td>
<p>CephObjectStoreZone name this CephObjectStore is part of</p>
</td>
</tr>
</tbody>
</table>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>.
</em></p>
