package sidecar

import (
	"os"
	"strings"
	"testing"
	"time"

	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	rookScheme "github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/operator/cassandra/constants"
	casstest "github.com/rook/rook/pkg/operator/cassandra/test"
	"k8s.io/client-go/kubernetes/fake"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestSidecar(t *testing.T) {
	namespace := "test-ns"
	memberName := "mymember"
	clusterName := "test-cluster"
	rackName := "test-rack"
	dcName := "test-dc"

	clientset := fake.NewSimpleClientset([]runtime.Object{
		&v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      memberName,
				Namespace: namespace,
				Labels: map[string]string{
					constants.SeedLabel:        "",
					constants.ClusterNameLabel: clusterName,
				},
			},
			Spec: v1.ServiceSpec{
				ClusterIP: "100.10.10.31",
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      memberName,
				Namespace: namespace,
				Labels: map[string]string{
					constants.ClusterNameLabel:    clusterName,
					constants.RackNameLabel:       rackName,
					constants.DatacenterNameLabel: dcName,
				},
			},
			Spec: v1.PodSpec{},
		},
		&v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: namespace,
			},
			Data: map[string]string{
				"disk_failure_policy": "die",
			},
		},
	}...)
	cluster := casstest.NewSimpleCluster(2)
	rookScheme.AddToScheme(scheme.Scheme)
	rookObjects := []runtime.Object{cluster}
	rookClient := rookfake.NewSimpleClientset(rookObjects...)
	kubeSharedInformerFactory := kubeinformers.NewSharedInformerFactory(clientset, time.Millisecond)
	serviceInformer := kubeSharedInformerFactory.Core().V1().Services()

	prevVal := os.Getenv(constants.PodIPEnvVar)
	os.Setenv(constants.PodIPEnvVar, "1.2.3.4")
	defer func() {
		os.Setenv(constants.PodIPEnvVar, prevVal)
	}()

	mc, err := New(memberName, namespace, clientset, rookClient, serviceInformer)
	bt, err := mc.generateCassandraConfig()
	if err != nil {
		t.Errorf("unexpected error generating confis: %s", err)
	}

	expect := "disk_failure_policy: die"
	if !strings.Contains(string(bt), expect) {
		t.Errorf("config does not have expected value. expected %s, config:\n\n%s", expect, string(bt))
	}
	expect = "broadcast_address: 100.10.10.31"
	if !strings.Contains(string(bt), expect) {
		t.Errorf("config does not have expected value. expected %s, config:\n\n%s", expect, string(bt))
	}
	expect = "listen_address: 1.2.3.4"
	if !strings.Contains(string(bt), expect) {
		t.Errorf("config does not have expected value. expected %s, config:\n\n%s", expect, string(bt))
	}
}
