/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"testing"

	"github.com/rook/rook/tests/framework/clients"
	"github.com/rook/rook/tests/framework/contracts"
	"github.com/rook/rook/tests/framework/enums"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// Rook Block Storage integration test
// Start MySql database that is using rook provisoned block storage.
// Make sure database is functional

func TestK8sBlockIntegration(t *testing.T) {
	suite.Run(t, new(K8sBlockEnd2EndIntegrationSuite))
}

type K8sBlockEnd2EndIntegrationSuite struct {
	suite.Suite
	testClient       *clients.TestClient
	bc               contracts.BlockOperator
	kh               *utils.K8sHelper
	initBlockCount   int
	storageclassPath string
	mysqlappPath     string
	db               *utils.MySQLHelper
	installer        *installer.InstallHelper
}

//Test set up - does the following in order
//create pool and storage class, create a PVC, Create a MySQL app/service that uses pvc
func (s *K8sBlockEnd2EndIntegrationSuite) SetupSuite() {

	var err error
	s.kh, err = utils.CreateK8sHelper()
	assert.Nil(s.T(), err)

	s.installer = installer.NewK8sRookhelper(s.kh.Clientset)

	err = s.installer.InstallRookOnK8s("rook")
	require.NoError(s.T(), err)

	s.testClient, err = clients.CreateTestClient(enums.Kubernetes, s.kh, "rook")
	require.Nil(s.T(), err)

	s.bc = s.testClient.GetBlockClient()
	initialBlocks, err := s.bc.BlockList()
	require.Nil(s.T(), err)
	s.initBlockCount = len(initialBlocks)

	s.storageclassPath = `apiVersion: rook.io/v1alpha1
kind: Pool
metadata:
  name: {{.poolName}}
  namespace: rook
spec:
  replicated:
    size: 1
  # For an erasure-coded pool, comment out the replication count above and uncomment the following settings.
  # Make sure you have enough OSDs to support the replica count or erasure code chunks.
  #erasureCoded:
  #  codingChunks: 2
  #  dataChunks: 2
---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
   name: rook-block
provisioner: rook.io/block
parameters:
    pool: {{.poolName}}`

	s.mysqlappPath = `apiVersion: v1
kind: Service
metadata:
  name: mysql-app
  labels:
    app: mysqldb
spec:
  ports:
    - port: 3306
      targetPort: 3306
      protocol: TCP
      nodePort: 30003
  selector:
    app: mysqldb
    tier: mysql
  type: NodePort
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-pv-claim
  labels:
    app: mysqldb
spec:
  storageClassName: rook-block
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi
---
apiVersion: apps/v1beta1
kind: Deployment
metadata:
  name: mysql-app
  labels:
    app: mysqldb
spec:
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: mysqldb
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
          claimName: mysql-pv-claim`

	//create storage class
	_, err = s.storageClassOperation("mysql-pool", "create")
	require.NoError(s.T(), err)

	//make sure storageclass is created
	present, err := s.kh.IsStorageClassPresent("rook-block")
	require.Nil(s.T(), err)
	require.True(s.T(), present, "Make sure storageclass is present")

	//create mysql pod
	s.kh.ResourceOperation("create", s.mysqlappPath)

	//wait till mysql pod is up

	require.True(s.T(), s.kh.IsPodInExpectedState("mysqldb", "", "Running"))
	pvcStatus, err := s.kh.GetPVCStatus("mysql-pv-claim")
	require.Nil(s.T(), err)
	require.Contains(s.T(), pvcStatus, "Bound")

	dbIP, err := s.kh.GetPodHostID("mysqldb", "")
	require.Nil(s.T(), err)
	//create database connection
	s.db = utils.CreateNewMySQLHelper("mysql", "mysql", dbIP+":30003", "sample")

}

func (s *K8sBlockEnd2EndIntegrationSuite) TestBlockE2EIntegrationWithMySqlDatabase() {

	//ping database
	require.True(s.T(), s.db.PingSuccess())

	//Create  a table
	s.db.CreateTable()
	require.EqualValues(s.T(), 0, s.db.TableRowCount(), "make sure tables has no rows initially")

	//Write Data
	s.db.InsertRandomData()
	require.EqualValues(s.T(), 1, s.db.TableRowCount(), "make sure new row is created")

	//delete Data
	s.db.DeleteRandomRow()
	require.EqualValues(s.T(), 0, s.db.TableRowCount(), "make sure row is deleted")

}

func (s *K8sBlockEnd2EndIntegrationSuite) storageClassOperation(poolName string, action string) (string, error) {
	config := map[string]string{
		"poolName": poolName,
	}

	result, err := s.kh.ResourceOperationFromTemplate(action, s.storageclassPath, config)

	return result, err
}

func (s *K8sBlockEnd2EndIntegrationSuite) TearDownTest() {

	s.kh.ResourceOperation("delete", s.mysqlappPath)
	s.storageClassOperation("mysql-pool", "delete")

}

func (s *K8sBlockEnd2EndIntegrationSuite) TearDownSuite() {

	s.installer.UninstallRookFromK8s("rook", false)
}
