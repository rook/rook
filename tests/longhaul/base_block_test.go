package longhaul

import (
	"sync"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"time"
)

var (
	logger           = capnslog.NewPackageLogger("github.com/rook/rook", "longhaul")
	defaultNamespace = "default"
)

func setUpRookAndPoolInNamespace(t func() *testing.T, namespace string, storageClassName string, poolName string) (*utils.K8sHelper, *installer.InstallHelper) {
	kh, err := utils.CreateK8sHelper(t)
	assert.Nil(t(), err)

	i := installer.NewK8sRookhelper(kh.Clientset, t)
	if !kh.IsRookInstalled(namespace) {
		isRookInstalled, err := i.InstallRookOnK8sWithHostPathAndDevices(namespace, "bluestore", "/temp/rookBackup", true, 3)
		require.NoError(t(), err)
		require.True(t(), isRookInstalled)
	}

	//create storage class
	if scp, _ := kh.IsStorageClassPresent(storageClassName); !scp {

		installer.BlockResourceOperation(kh, installer.GetBlockStorageClassDef(poolName, storageClassName, namespace), "create")

		logger.Infof("Install pool and storage class for rook block")
		_, err := installer.BlockResourceOperation(kh, installer.GetBlockPoolDef(poolName, namespace, "3"), "create")
		require.NoError(t(), err)
		_, err = installer.BlockResourceOperation(kh, installer.GetBlockStorageClassDef(poolName, storageClassName, namespace), "create")
		require.NoError(t(), err)

		//make sure storageclass is created
		present, err := kh.IsStorageClassPresent(storageClassName)
		require.NoError(t(), err)
		require.True(t(), present, "Make sure storageclass is present")
	}
	return kh, i
}

// Set Up rook, storageClass ,pvc and mysql pods for longhaul test
// All the set up is if needed.
func createPVCAndMountMysqlPod(t func() *testing.T, kh *utils.K8sHelper, storageClassName string, appName string, appLabel string, pvcName string) *utils.MySQLHelper {

	//create mysql pod
	if _, err := kh.GetPVCStatus(defaultNamespace, pvcName); err != nil {
		logger.Infof("Create PVC")

		mySqlPodOperation(kh, storageClassName, appName, appLabel, pvcName, "create")

		//wait till mysql pod is up
		require.True(t(), kh.IsPodInExpectedState(appLabel, "", "Running"))
		require.True(t(), kh.WaitUntilPVCIsBound(defaultNamespace, pvcName))
	}
	dbIP, err := kh.GetPodHostID(appLabel, "")
	require.Nil(t(), err)
	dbPort, err := kh.GetServiceNodePort(appName, "default")
	require.Nil(t(), err)
	//create database connection
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

	result, err := k8sh.ResourceOperationFromTemplate(action, getBlockPodDefintion(), config)

	return result, err
}

func performBlockOperations(installer *installer.InstallHelper, db *utils.MySQLHelper) {
	var wg sync.WaitGroup
	for i := 1; i <= installer.Env.LoadConcurrentRuns; i++ {
		wg.Add(1)
		go dbOperation(db, &wg, installer.Env.LoadTime)
	}
	wg.Wait()
}

func dbOperation(db *utils.MySQLHelper, wg *sync.WaitGroup, runtime int) {
	defer wg.Done()
	start := time.Now()
	elapsed := time.Since(start).Seconds()
	for elapsed < float64(runtime) {
		//InsertRandomData
		db.InsertRandomData()
		db.InsertRandomData()
		db.InsertRandomData()
		db.SelectRandomData(5)
		db.InsertRandomData()
		db.InsertRandomData()
		db.InsertRandomData()
		db.SelectRandomData(10)

		//delete Data
		db.DeleteRandomRow()
		db.SelectRandomData(20)
		elapsed = time.Since(start).Seconds()
	}

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
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: {{.appName}}
  labels:
    app: {{.appLabel}}
spec:
  strategy:
    type: Recreate
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

func getBlockPodDefintion() string {
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
          mountPath: /tmp/rook1
      volumes:
      - name: block-persistent-storage
        persistentVolumeClaim:
          claimName: {{.pvcName}}
          readOnly: {{.readOnly}}
      restartPolicy: Never`
}
