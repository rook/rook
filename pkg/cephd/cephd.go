package cephd

// #cgo CFLAGS: -I${SRCDIR}/../../ceph/src/include
// #cgo jemalloc,static LDFLAGS: -ljemalloc
// #cgo tcmalloc,static LDFLAGS: -ltcmalloc_minimal
// #cgo jemalloc,dynamic LDFLAGS: -Wl,-Bstatic -ljemalloc -Wl,-Bdynamic
// #cgo tcmalloc,dynamic LDFLAGS: -Wl,-Bstatic -ltcmalloc_minimal -Wl,-Bdynamic
// #cgo jemalloc tcmalloc CFLAGS: -fno-builtin-malloc -fno-builtin-calloc -fno-builtin-realloc -fno-builtin-free
// #cgo jemalloc tcmalloc CXXFLAGS: -fno-builtin-malloc -fno-builtin-calloc -fno-builtin-realloc -fno-builtin-free
// #cgo static LDFLAGS: -L${SRCDIR}/../../ceph/build/lib -lcephd -lboost_system -lboost_thread -lboost_iostreams -lboost_random -lz -lsnappy -lcrypto++ -lleveldb -laio -lblkid -luuid -lm -ldl -lresolv
// #cgo dynamic LDFLAGS: -L${SRCDIR}/../../ceph/build/lib -Wl,-Bstatic -lcephd -lboost_system -lboost_thread -lboost_iostreams -lboost_random -lz -lsnappy -lcrypto++ -lleveldb -laio -lblkid -luuid -Wl,-Bdynamic -ldl -lm -lresolv
// #include <errno.h>
// #include <stdlib.h>
// #include <string.h>
// #include "cephd/libcephd.h"
// #include "rados/librados.h"
import "C"

import (
	"fmt"
	"os"
	"unsafe"
)

// Version returns the major, minor, and patch components of the version of
// the RADOS library linked against.
func RadosVersion() (int, int, int) {
	var c_major, c_minor, c_patch C.int
	C.rados_version(&c_major, &c_minor, &c_patch)
	return int(c_major), int(c_minor), int(c_patch)
}

// cephdError represents an error
type cephdError int

// Error returns a formatted error string
func (e cephdError) Error() string {
	return fmt.Sprintf("cephd: %s", C.GoString(C.strerror(C.int(-e))))
}

// Version returns the version of Ceph
func Version() string {
	var cMajor, cMinor, cPatch C.int
	return C.GoString(C.ceph_version(&cMajor, &cMinor, &cPatch))
}

// NewFsid generates a new cluster id
func NewFsid() (string, error) {
	buf := make([]byte, 37)
	ret := int(C.cephd_generate_fsid((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	if ret >= 0 {
		return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
	}

	return "", cephdError(int(ret))
}

// NewSecretKey generates a new secret key
func NewSecretKey() (string, error) {
	buf := make([]byte, 128)
	ret := int(C.cephd_generate_secret_key((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	if ret >= 0 {
		return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
	}

	return "", cephdError(int(ret))
}

// Mon runs embedded ceph-mon.
func RunDaemon(daemon string, args ...string) error {

	// BUGBUG: the first arg is really not needed but its an artifact
	// of calling ceph-mon.main(). Should be removed on the C++ side.

	finalArgs := append([]string{os.Args[0]}, args...)

	var cptr *C.char
	ptrSize := unsafe.Sizeof(cptr)

	// Allocate the char** list.
	ptr := C.malloc(C.size_t(len(finalArgs)) * C.size_t(ptrSize))
	defer C.free(ptr)

	// Assign each byte slice to its appropriate offset.
	for i := 0; i < len(finalArgs); i++ {
		element := (**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*ptrSize))
		*element = C.CString(finalArgs[i])
		defer C.free(unsafe.Pointer(*element))
	}

	var ret C.int

	if daemon == "mon" {
		ret = C.cephd_mon(C.int(len(finalArgs)), (**C.char)(ptr))
	} else if daemon == "osd" {
		ret = C.cephd_osd(C.int(len(finalArgs)), (**C.char)(ptr))
	}
	if ret < 0 {
		return cephdError(int(ret))
	}

	return nil
}

// TODO: the below rados_connect and rados_mon_command is from go-ceph: https://github.com/ceph/go-ceph
// If it stays, then applicable LICENSE needs to be added.  A better approach would be to create a
// wrapper in libcephd around MonCommand, and remove this go-ceph code and embedded librados altogether.

// Conn is a connection handle to a Ceph cluster.
type Conn struct {
	cluster C.rados_t
}

// NewConnWithClusterAndUser creates a new connection object for a specific cluster and username.
// It returns the connection and an error, if any.
func NewConnWithClusterAndUser(clusterName string, userName string) (*Conn, error) {
	c_cluster_name := C.CString(clusterName)
	defer C.free(unsafe.Pointer(c_cluster_name))

	c_name := C.CString(userName)
	defer C.free(unsafe.Pointer(c_name))

	conn := &Conn{}
	ret := C.rados_create2(&conn.cluster, c_cluster_name, c_name, 0)
	if ret == 0 {
		return conn, nil
	} else {
		return nil, cephdError(int(ret))
	}
}

// Connect establishes a connection to a RADOS cluster. It returns an error,
// if any.
func (c *Conn) Connect() error {
	ret := C.rados_connect(c.cluster)
	if ret == 0 {
		return nil
	} else {
		return cephdError(int(ret))
	}
}

// Shutdown disconnects from the cluster.
func (c *Conn) Shutdown() {
	C.rados_shutdown(c.cluster)
}

// ReadConfigFile configures the connection using a Ceph configuration file.
func (c *Conn) ReadConfigFile(path string) error {
	c_path := C.CString(path)
	defer C.free(unsafe.Pointer(c_path))
	ret := C.rados_conf_read_file(c.cluster, c_path)
	if ret == 0 {
		return nil
	} else {
		return cephdError(int(ret))
	}
}

// MonCommand sends a command to one of the monitors
func (c *Conn) MonCommand(args []byte) (buffer []byte, info string, err error) {
	return c.monCommand(args, nil)
}

// MonCommand sends a command to one of the monitors, with an input buffer
func (c *Conn) MonCommandWithInputBuffer(args, inputBuffer []byte) (buffer []byte, info string, err error) {
	return c.monCommand(args, inputBuffer)
}

func (c *Conn) monCommand(args, inputBuffer []byte) (buffer []byte, info string, err error) {
	argv := C.CString(string(args))
	defer C.free(unsafe.Pointer(argv))

	var (
		outs, outbuf       *C.char
		outslen, outbuflen C.size_t
	)
	inbuf := C.CString(string(inputBuffer))
	inbufLen := len(inputBuffer)
	defer C.free(unsafe.Pointer(inbuf))

	ret := C.rados_mon_command(c.cluster,
		&argv, 1,
		inbuf,              // bulk input (e.g. crush map)
		C.size_t(inbufLen), // length inbuf
		&outbuf,            // buffer
		&outbuflen,         // buffer length
		&outs,              // status string
		&outslen)

	if outslen > 0 {
		info = C.GoStringN(outs, C.int(outslen))
		C.free(unsafe.Pointer(outs))
	}
	if outbuflen > 0 {
		buffer = C.GoBytes(unsafe.Pointer(outbuf), C.int(outbuflen))
		C.free(unsafe.Pointer(outbuf))
	}
	if ret != 0 {
		err = cephdError(int(ret))
		return nil, info, err
	}

	return
}
