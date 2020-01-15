package longhaul

import (
	"strings"
	"sync"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/require"
)

var (
	defaultNamespace = "default"
)

// Create StorageClass and poll if needed
func createStorageClassAndPool(t func() *testing.T, testClient *clients.TestClient, kh *utils.K8sHelper, namespace string, storageClassName string, poolName string) {
	// Create storage class
	if err := kh.IsStorageClassPresent(storageClassName); err != nil {
		logger.Infof("Install pool and storage class for rook block")
		err := testClient.PoolClient.Create(poolName, namespace, 3)
		require.NoError(t(), err)
		err = testClient.BlockClient.CreateStorageClass(poolName, storageClassName, "Delete", namespace, false)
		require.NoError(t(), err)

		// make sure storageclass is created
		err = kh.IsStorageClassPresent(storageClassName)
		require.NoError(t(), err)
	}
}

// Set Up rook, storageClass ,pvc and mysql pods for longhaul test
// All the set up is if needed.
func createPVCAndMountMysqlPod(t func() *testing.T, kh *utils.K8sHelper, storageClassName string, appName string, appLabel string, pvcName string) *utils.MySQLHelper {

	// Create mysql pod
	if _, err := kh.GetPVCStatus(defaultNamespace, pvcName); err != nil {
		logger.Infof("Create PVC")

		mySqlPodOperation(kh, storageClassName, appName, appLabel, pvcName, "apply")

		// Wait till mysql pod is up
		require.True(t(), kh.IsPodInExpectedState(appLabel, "", "Running"))
		require.True(t(), kh.WaitUntilPVCIsBound(defaultNamespace, pvcName))
	}
	dbIP, err := kh.GetPodHostID(appLabel, "")
	require.Nil(t(), err)
	dbPort, err := kh.GetServiceNodePort(appName, "default")
	require.Nil(t(), err)
	// Create database connection
	db := utils.CreateNewMySQLHelper("mysql", "mysql", dbIP+":"+dbPort, "sample")

	require.True(t(), db.PingSuccess())

	if exist := db.TableExists(); !exist {
		db.CreateTable()
	}

	return db
}

func mySqlPodOperation(k8sh *utils.K8sHelper, storageClassName string, appName string, appLabel string, pvcName string, action string) (string, error) {
	config := map[string]string{
		"appName":          appName,
		"appLabel":         appLabel,
		"storageClassName": storageClassName,
		"pvcName":          pvcName,
	}

	result, err := k8sh.ResourceOperationFromTemplate(action, GetMySqlPodDef(), config)
	return result, err

}

func mountUnmountPVCOnPod(k8sh *utils.K8sHelper, podName string, pvcName string, readonly string, action string) (string, error) {
	config := map[string]string{
		"podName":  podName,
		"pvcName":  pvcName,
		"readOnly": readonly,
	}

	result, err := k8sh.ResourceOperationFromTemplate(action, getBlockPodDefinition(), config)

	return result, err
}

func performBlockOperations(db *utils.MySQLHelper) {
	var wg sync.WaitGroup
	for i := 1; i <= installer.Env.LoadConcurrentRuns; i++ {
		wg.Add(1)
		go dbOperation(db, &wg, installer.Env.LoadTime, installer.Env.LoadSize)
	}
	wg.Wait()
}

func dbOperation(db *utils.MySQLHelper, wg *sync.WaitGroup, runtime int, loadSize string) {
	defer wg.Done()
	ds := 100000
	switch strings.ToLower(loadSize) {
	case "small":
		ds = 105000 //.1M * 5 columns * 6 = 3M per thread
	case "medium":
		ds = 419430 // .4M * 5 columns * 6 = 12M per thread
	case "large":
		ds = 2100000 // 2M * 5 columns * 6 = 60M per thread
	default:
		ds = 209715 // .2M * 5 columns * 6 = 15M per thread
	}
	start := time.Now()
	elapsed := time.Since(start).Seconds()
	for elapsed < float64(runtime) {
		// InsertRandomData
		db.InsertRandomData(ds)
		db.InsertRandomData(ds)
		db.InsertRandomData(ds)
		db.SelectRandomData(5)
		db.InsertRandomData(ds)
		db.InsertRandomData(ds)
		db.InsertRandomData(ds)
		db.SelectRandomData(10)

		// Delete Data
		db.DeleteRandomRow()
		db.SelectRandomData(20)
		elapsed = time.Since(start).Seconds()
	}

}

// LoadTestCluster struct for handling panic and test suite tear down
type LoadTestCluster struct {
	installer *installer.CephInstaller
	kh        *utils.K8sHelper
	helper    *clients.TestClient
	T         func() *testing.T
	namespace string
}

// StartLoadTestCluster creates new instance of TestCluster struct
func StartLoadTestCluster(t func() *testing.T, namespace string) (LoadTestCluster, *utils.K8sHelper, *installer.CephInstaller) {
	kh, err := utils.CreateK8sHelper(t)
	require.NoError(t(), err)

	i := installer.NewCephInstaller(t, kh.Clientset, false, installer.VersionMaster, cephv1.CephVersionSpec{Image: "ceph/ceph:v14.2.6"})

	op := LoadTestCluster{i, kh, nil, t, namespace}
	op.Setup()
	return op, kh, i
}

// Setup is a wrapper for setting up rook
func (o LoadTestCluster) Setup() {

	if !o.kh.IsRookInstalled(o.namespace) {
		isRookInstalled, err := o.installer.InstallRookOnK8sWithHostPathAndDevices(o.namespace, "bluestore",
			true, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true},
			true, /* startWithAllNodes */
			1 /*rbd mirror workers*/)
		require.NoError(o.T(), err)
		require.True(o.T(), isRookInstalled)
	}

	// Enable chaos monkey if enable_chaos flag is present
	if installer.Env.EnableChaos {
		c := NewChaosHelper(o.namespace, o.kh)
		go c.Monkey()
	}
}

// TearDownRook is a wrapper for tearDown after suite
func (o LoadTestCluster) Teardown() {
	// No Clean up for load test
}

func GetMySqlPodDef() string {
	return `apiVersion: v1
kind: Service
metadata:
  name: {{.appName}}
  labels:
    app: {{.appLabel}}
spec:
  ports:
    - port: 3306
      targetPort: 3306
      protocol: TCP
  selector:
    app: {{.appLabel}}
    tier: mysql
  type: NodePort
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{.pvcName}}
  labels:
    app: {{.appLabel}}
spec:
  storageClassName: {{.storageClassName}}
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.appName}}
  labels:
    app: {{.appLabel}}
spec:
  strategy:
    type: Recreate
  selector:
    matchLabels:
      app: {{.appLabel}}
  template:
    metadata:
      labels:
        app: {{.appLabel}}
        tier: mysql
    spec:
      containers:
      - image: mysql:5.6
        name: mysql
        env:
        - name: "MYSQL_USER"
          value: "mysql"
        - name: "MYSQL_PASSWORD"
          value: "mysql"
        - name: "MYSQL_DATABASE"
          value: "sample"
        - name: "MYSQL_ROOT_PASSWORD"
          value: "root"
        ports:
        - containerPort: 3306
          name: mysql
        volumeMounts:
        - name: mysql-persistent-storage
          mountPath: /var/lib/mysql
      volumes:
      - name: mysql-persistent-storage
        persistentVolumeClaim:
          claimName: {{.pvcName}}`
}

func getBlockPodDefinition() string {
	return `apiVersion: v1
kind: Pod
metadata:
  name: {{.podName}}
spec:
      containers:
      - image: busybox
        name: {{.podName}}
        command:
          - sleep
          - "3600"
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - name: block-persistent-storage
          mountPath: ` + utils.TestMountPath + `
      volumes:
      - name: block-persistent-storage
        persistentVolumeClaim:
          claimName: {{.pvcName}}
          readOnly: {{.readOnly}}
      restartPolicy: Never`
}
