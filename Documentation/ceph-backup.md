---
title: Back Up
weight: 2200
indent: true
---

# Back Up

Back Up of Ceph helps you recover your data when a disaster occurs.

## Requirements

The following requirements are needed:

1. Rook Ceph [PVC-based Cluster](ceph-cluster-crd.md#pvc-based-cluster).
2. [Velero](https://velero.io/) + [Restic](https://velero.io/docs/v1.5/restic/)

## Deploy of a Ceph PVC-based Cluster

### Deploy the operator

The first step is to deploy the Rook operator. Check that you are using the [example yaml files](https://github.com/rook/rook/blob/{{ branchName }}/cluster/examples/kubernetes/ceph) that correspond to your release of Rook. For more options, see the [examples documentation](ceph-examples.md).

```console
cd cluster/examples/kubernetes/ceph
kubectl create -f common.yaml
kubectl create -f operator.yaml

## verify the rook-ceph-operator is in the `Running` state before proceeding
kubectl -n rook-ceph get pod
```

You can also deploy the operator with the [Rook Helm Chart](helm-operator.md).

### Create a Rook Ceph PVC-based Cluster

A Ceph PVC-based Cluster stores the `MONs` and `OSDs` data in PVCs.

We can create the Ceph based cluster as follow:

```console
kubectl create -f cluster-on-pvc.yaml
```

In that yaml file you need to adjust the `storageClassName` to match the one from your provider. For example, for GKE is `standard`:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
    # A volume claim template can be specified in which case new monitors (and
    # monitors created during fail over) will construct a PVC based on the
    # template for the monitor's primary storage. Changes to the template do not
    # affect existing monitors. Log data is stored on the HostPath under
    # dataDirHostPath. If no storage requirement is specified, a default storage
    # size appropriate for monitor data will be used.
    volumeClaimTemplate:
      spec:
        storageClassName: standard
        resources:
          requests:
            storage: 10Gi
  cephVersion:
    image: ceph/ceph:v15.2.4
    allowUnsupported: false
  skipUpgradeChecks: false
  continueUpgradeAfterChecksEvenIfNotHealthy: false
  mgr:
    modules:
    - name: pg_autoscaler
      enabled: true
  dashboard:
    enabled: true
    ssl: true
  crashCollector:
    disable: false
  storage:
    storageClassDeviceSets:
    - name: set1
      # The number of OSDs to create from this device set
      count: 3
      # IMPORTANT: If volumes specified by the storageClassName are not portable across nodes
      # this needs to be set to false. For example, if using the local storage provisioner
      # this should be false.
      portable: true
      # Certain storage class in the Cloud are slow
      # Rook can configure the OSD running on PVC to accommodate that by tuning some of the Ceph internal
      # Currently, "gp2" has been identified as such
      tuneDeviceClass: true
      # whether to encrypt the deviceSet or not
      encrypted: false
      # Since the OSDs could end up on any node, an effort needs to be made to spread the OSDs
      # across nodes as much as possible. Unfortunately the pod anti-affinity breaks down
      # as soon as you have more than one OSD per node. The topology spread constraints will
      # give us an even spread on K8s 1.18 or newer.
      placement:
        topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchExpressions:
            - key: app
              operator: In
              values:
              - rook-ceph-osd
      preparePlacement:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - rook-ceph-osd
                - key: app
                  operator: In
                  values:
                  - rook-ceph-osd-prepare
              topologyKey: kubernetes.io/hostname
        topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchExpressions:
            - key: app
              operator: In
              values:
              - rook-ceph-osd-prepare
      resources:
      # These are the OSD daemon limits. For OSD prepare limits, see the separate section below for "prepareosd" resources
      #   limits:
      #     cpu: "500m"
      #     memory: "4Gi"
      #   requests:
      #     cpu: "500m"
      #     memory: "4Gi"
      volumeClaimTemplates:
      - metadata:
          name: data
          # if you are looking at giving your OSD a different CRUSH device class than the one detected by Ceph
          # annotations:
          #   crushDeviceClass: hybrid
        spec:
          resources:
            requests:
              storage: 40Gi
          # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, gp2)
          storageClassName: standard
          volumeMode: Block
          accessModes:
            - ReadWriteOnce
      # dedicated block device to store bluestore database (block.db)
      # - metadata:
      #     name: metadata
      #   spec:
      #     resources:
      #       requests:
      #         # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
      #         storage: 5Gi
      #     # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
      #     storageClassName: io1
      #     volumeMode: Block
      #     accessModes:
      #       - ReadWriteOnce
      # dedicated block device to store bluestore wal (block.wal)
      # - metadata:
      #     name: wal
      #   spec:
      #     resources:
      #       requests:
      #         # Find the right size https://docs.ceph.com/docs/master/rados/configuration/bluestore-config-ref/#sizing
      #         storage: 5Gi
      #     # IMPORTANT: Change the storage class depending on your environment (e.g. local-storage, io1)
      #     storageClassName: io1
      #     volumeMode: Block
      #     accessModes:
      #       - ReadWriteOnce
      # Scheduler name for OSD pod placement
      # schedulerName: osd-scheduler
  resources:
  #  prepareosd:
  #    limits:
  #      cpu: "200m"
  #      memory: "200Mi"
  #   requests:
  #      cpu: "200m"
  #      memory: "200Mi"
  disruptionManagement:
    managePodBudgets: false
    osdMaintenanceTimeout: 30
    manageMachineDisruptionBudgets: false
    machineDisruptionBudgetNamespace: openshift-machine-api
```

### Create an Object Store to confirm that data is restored

We are going to create an Object Store, create a bucket and add some items. So, on restore we can confirm that all data is restored.

Here are the commands. However for a deeper tutorial go to [Ceph Object Storage documentation](ceph-object.md).

The Bucket Claim definition is as follows:

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-delete-bucket
  namespace: rook-ceph
spec:
  # To create a new bucket specify either `bucketName` or 
  # `generateBucketName` here. Both cannot be used. To access
  # an existing bucket the bucket name needs to be defined in
  # the StorageClass referenced here, and both `bucketName` and
  # `generateBucketName` must be omitted in the OBC.
  #bucketName: 
  bucketName: ceph-bkt
  storageClassName: rook-ceph-delete-bucket
  additionalConfig:
    # To set for quota for OBC
    #maxObjects: "1000"
    #maxSize: "2G"
```

Execute the following commands in order:

```console
# Create Object Store
kubectl apply -f object.yaml

# Bucket Creation
kubectl apply -f storageclass-bucket-delete.yaml
kubectl apply -f object-bucket-claim-delete.yaml

# Connect to Bucket, execute at background in new terminal
kubectl port-forward -n rook-ceph service/rook-ceph-rgw-my-store 8080:80

# Add data
export AWS_HOST=`kubectl get configmap -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.BUCKET_HOST}';echo`
export AWS_ENDPOINT=localhost:8080
export BUCKET_NAME=`kubectl get configmap -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.BUCKET_NAME}';echo`
export AWS_ACCESS_KEY_ID=`kubectl get secret -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.AWS_ACCESS_KEY_ID}' | base64 --decode;echo`
export AWS_SECRET_ACCESS_KEY=`kubectl get secret -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.AWS_SECRET_ACCESS_KEY}' | base64 --decode;echo`

for i in {1..10};do aws s3 cp /etc/hosts s3://${BUCKET_NAME}/$i --endpoint-url http://${AWS_ENDPOINT};done

# List data
aws s3 ls s3://${BUCKET_NAME} --endpoint-url http://${AWS_ENDPOINT}
```

## Install and configure Velero

[Velero](https://velero.io/) is an open source tool to safely backup and restore, perform disaster recovery, and migrate Kubernetes cluster resources and persistent volumes.

For backing up and restoring Kubernetes volumes you need to use the [restic plugin](https://velero.io/docs/v1.5/restic/). You need to add `--use-restic` to velero install command.

Velero supports a lot of cloud providers. Take at look in the [supported providers](https://velero.io/docs/v1.5/supported-providers/) section of the doc for more information and examples. 

For example for [GCP](https://github.com/vmware-tanzu/velero-plugin-for-gcp#setup).

```console

# Configure Google Storage bucket and service account.
BUCKET=<YOUR_BUCKET>
gsutil mb gs://$BUCKET/

PROJECT_ID=$(gcloud config get-value project)

gcloud iam service-accounts create velero \
    --display-name "Velero service account"

SERVICE_ACCOUNT_EMAIL=$(gcloud iam service-accounts list \
  --filter="displayName:Velero service account" \
  --format 'value(email)')

ROLE_PERMISSIONS=(
    compute.disks.get
    compute.disks.create
    compute.disks.createSnapshot
    compute.snapshots.get
    compute.snapshots.create
    compute.snapshots.useReadOnly
    compute.snapshots.delete
    compute.zones.get
)

gcloud iam roles create velero.server \
    --project $PROJECT_ID \
    --title "Velero Server" \
    --permissions "$(IFS=","; echo "${ROLE_PERMISSIONS[*]}")"

gcloud projects add-iam-policy-binding $PROJECT_ID \
    --member serviceAccount:$SERVICE_ACCOUNT_EMAIL \
    --role projects/$PROJECT_ID/roles/velero.server

gcloud iam service-accounts keys create credentials-velero \
    --iam-account $SERVICE_ACCOUNT_EMAIL

# Install velero
velero install --provider gcp --plugins velero/velero-plugin-for-gcp:v1.1.0 --bucket $BUCKET --secret-file ./credentials-velero --use-restic

```

## Create Back Up

We need to save Volumes, Volumes Claims, ConfigMaps, Secrets and Services.

```console
velero backup create <BackupName> --include-namespaces rook-ceph --include-resources pv,pvc,services,configmaps,secrets --default-volumes-to-restic
```

**IMPORTANT:** make sure to back up the bucket's configmaps and secrets that are located in a different namespace.

As Velero change ClusterIP from `MON` services on restore, we need to backup the `yamls` so `MON` can form quorum again.

```console
kubectl get svc -n rook-ceph rook-ceph-mon-a -o yaml > mon-service-a.yaml
kubectl get svc -n rook-ceph rook-ceph-mon-b -o yaml > mon-service-b.yaml
kubectl get svc -n rook-ceph rook-ceph-mon-c -o yaml > mon-service-c.yaml
```

For each of those files we have to delete `metadata.creationTimestamp`, `metadata.ownerReferences`, `metadata.resourceVersion`, `metadata.selfLink` and `metadata.uid`.

For example for file `mon-service-a.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2020-10-04T19:22:31Z"
  labels:
    app: rook-ceph-mon
    ceph_daemon_id: a
    ceph_daemon_type: mon
    mon: a
    mon_cluster: rook-ceph
    pvc_name: rook-ceph-mon-a
    rook_cluster: rook-ceph
  name: rook-ceph-mon-a
  namespace: rook-ceph
  ownerReferences:
  - apiVersion: ceph.rook.io/v1
    blockOwnerDeletion: true
    controller: true
    kind: CephCluster
    name: rook-ceph
    uid: 7561619e-bd35-4649-8af7-a7a0dd7c11b4
  resourceVersion: "3046"
  selfLink: /api/v1/namespaces/rook-ceph/services/rook-ceph-mon-a
  uid: 77e161de-d9ed-41eb-b9c2-386393d21369
spec:
  clusterIP: 10.4.1.210
  ports:
  - name: tcp-msgr1
    port: 6789
    protocol: TCP
    targetPort: 6789
  - name: tcp-msgr2
    port: 3300
    protocol: TCP
    targetPort: 3300
  selector:
    app: rook-ceph-mon
    ceph_daemon_id: a
    mon: a
    mon_cluster: rook-ceph
    rook_cluster: rook-ceph
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
```

We end up with:
```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    app: rook-ceph-mon
    ceph_daemon_id: a
    ceph_daemon_type: mon
    mon: a
    mon_cluster: rook-ceph
    pvc_name: rook-ceph-mon-a
    rook_cluster: rook-ceph
  name: rook-ceph-mon-a
  namespace: rook-ceph
spec:
  clusterIP: 10.4.1.210
  ports:
  - name: tcp-msgr1
    port: 6789
    protocol: TCP
    targetPort: 6789
  - name: tcp-msgr2
    port: 3300
    protocol: TCP
    targetPort: 3300
  selector:
    app: rook-ceph-mon
    ceph_daemon_id: a
    mon: a
    mon_cluster: rook-ceph
    rook_cluster: rook-ceph
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}
```

## Restore Back Up

Execute the following command to restore the backup.

```console
velero restore create --from-backup <BackupName>
```

Delete the `MON` services backed up by Velero and restore the old ones.

```console
# Delete MONs
kubectl delete svc -n rook-ceph rook-ceph-mon-a
kubectl delete svc -n rook-ceph rook-ceph-mon-b
kubectl delete svc -n rook-ceph rook-ceph-mon-c

# Restore the new ones
kubectl apply -f mon-service-a.yaml
kubectl apply -f mon-service-b.yaml
kubectl apply -f mon-service-c.yaml
```

After that we need to create cluster.

```console
kubectl apply -f common.yaml
kubectl apply -f operator.yaml
kubectl apply -f cluster-on-pvc.yaml
```

Owner reference will be attached to `MON` services.

Now we can deploy the object store and check for the added data:

```console
# Create Object Store
kubectl apply -f object.yaml

# Bucket Creation
kubectl apply -f storageclass-bucket-delete.yaml
kubectl apply -f object-bucket-claim-delete.yaml

# Connect to Bucket, execute at background in new terminal
kubectl port-forward -n rook-ceph service/rook-ceph-rgw-my-store 8080:80

export AWS_HOST=`kubectl get configmap -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.BUCKET_HOST}';echo`
export AWS_ENDPOINT=localhost:8080
export BUCKET_NAME=`kubectl get configmap -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.BUCKET_NAME}';echo`
export AWS_ACCESS_KEY_ID=`kubectl get secret -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.AWS_ACCESS_KEY_ID}' | base64 --decode;echo`
export AWS_SECRET_ACCESS_KEY=`kubectl get secret -n rook-ceph ceph-delete-bucket -o 'jsonpath={.data.AWS_SECRET_ACCESS_KEY}' | base64 --decode;echo`

# List data
aws s3 ls s3://${BUCKET_NAME} --endpoint-url http://${AWS_ENDPOINT}
```

### New Cluster

The previous explanation will work with new clusters too. You just need to install velero first and make sure that **service clusterip range is the same as in the previous cluster**.

For example, when creating a GKE cluster from CLI use the flag `--services-ipv4-cidr`. More info [here](https://cloud.google.com/kubernetes-engine/docs/how-to/alias-ips).

In our previous example, `--services-ipv4-cidr=10.4.0.0/16`.