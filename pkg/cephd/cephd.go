package cephd

// #cgo CFLAGS: -I${SRCDIR}/../../ceph/src/include
// #cgo jemalloc LDFLAGS: -ljemalloc
// #cgo tcmalloc LDFLAGS: -ltcmalloc_minimal
// #cgo LDFLAGS: -L${SRCDIR}/../../ceph/build/lib -lcephd -lm -ldl -lboost_system -lboost_thread -lboost_iostreams -lboost_random -lz -lsnappy -lcrypto++ -lresolv -lleveldb
// #cgo jemalloc tcmalloc CFLAGS: -fno-builtin-malloc -fno-builtin-calloc -fno-builtin-realloc -fno-builtin-free
// #cgo jemalloc tcmalloc CXXFLAGS: -fno-builtin-malloc -fno-builtin-calloc -fno-builtin-realloc -fno-builtin-free
// #include <errno.h>
// #include <stdlib.h>
// #include <string.h>
// #include "cephd/libcephd.h"
import "C"

import "fmt"

// cephdError represents an error
type cephdError int

// Error returns a formatted error string
func (e cephdError) Error() string {
	return fmt.Sprintf("cephd: %s", C.GoString(C.strerror(C.int(-e))))
}

// Cluster bootstrap information
type Cluster struct {
	Fsid          string
	MonitorSecret string
	AdminSecret   string
}

// Version returns the version of Ceph
func Version() string {
	var cMajor, cMinor, cPatch C.int
	return C.GoString(C.ceph_version(&cMajor, &cMinor, &cPatch))
}

// NewCluster creates a new cluster
func NewCluster() (cluster Cluster, err error) {
	cCluster := C.struct_cephd_cluster_t{}
	ret := C.cephd_cluster_create(&cCluster)
	if ret < 0 {
		return Cluster{}, cephdError(int(ret))
	}
	return Cluster{
		Fsid:          C.GoString(&cCluster.fsid[0]),
		MonitorSecret: C.GoString(&cCluster.monitorSecret[0]),
		AdminSecret:   C.GoString(&cCluster.adminSecret[0]),
	}, nil
}
