package castled

import (
	"fmt"

	"github.com/quantum/castle/pkg/cephd"
)

func Start() {
	cluster, _ := cephd.NewCluster()
	fmt.Printf("%v %v %v", cluster.Fsid, cluster.MonitorSecret, cluster.AdminSecret)
}
