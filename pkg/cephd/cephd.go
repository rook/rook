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

	"github.com/quantum/castle/pkg/cephclient"
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

func GetCephdError(err int) error {
	if err == 0 {
		return nil
	}
	return cephdError(err)
}

// Error returns a formatted error string
func (e cephdError) Error() string {
	return fmt.Sprintf("cephd: %s", C.GoString(C.strerror(C.int(-e))))
}

func New() *ceph {
	return &ceph{}
}

type ceph struct {
}

// Version returns the version of Ceph
func Version() string {
	var cMajor, cMinor, cPatch C.int
	return C.GoString(C.ceph_version(&cMajor, &cMinor, &cPatch))
}

// NewFsid generates a new cluster id
func (c *ceph) NewFsid() (string, error) {
	buf := make([]byte, 37)
	ret := int(C.cephd_generate_fsid((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	if ret >= 0 {
		return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
	}

	return "", GetCephdError(int(ret))
}

// NewSecretKey generates a new secret key
func (c *ceph) NewSecretKey() (string, error) {
	buf := make([]byte, 128)
	ret := int(C.cephd_generate_secret_key((*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	if ret >= 0 {
		return C.GoString((*C.char)(unsafe.Pointer(&buf[0]))), nil
	}

	return "", GetCephdError(int(ret))
}

// Mon runs embedded ceph-mon.
func (c *ceph) RunDaemon(daemon string, args ...string) error {

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
		return GetCephdError(int(ret))
	}

	return nil
}

// TODO: the below rados_connect and rados_mon_command is from go-ceph: https://github.com/ceph/go-ceph
// If it stays, then applicable LICENSE needs to be added.  A better approach would be to create a
// wrapper in libcephd around MonCommand, and remove this go-ceph code and embedded librados altogether.

// conn is a connection handle to a Ceph cluster.
type conn struct {
	cluster C.rados_t
}

// NewConnWithClusterAndUser creates a new connection object for a specific cluster and username.
// It returns the connection and an error, if any.
func (c *ceph) NewConnWithClusterAndUser(clusterName string, userName string) (cephclient.Connection, error) {
	c_cluster_name := C.CString(clusterName)
	defer C.free(unsafe.Pointer(c_cluster_name))

	c_name := C.CString(userName)
	defer C.free(unsafe.Pointer(c_name))

	conn := &conn{}
	ret := C.rados_create2(&conn.cluster, c_cluster_name, c_name, 0)
	if ret == 0 {
		return conn, nil
	} else {
		return nil, GetCephdError(int(ret))
	}
}

// Connect establishes a connection to a RADOS cluster. It returns an error,
// if any.
func (c *conn) Connect() error {
	ret := C.rados_connect(c.cluster)
	if ret == 0 {
		return nil
	} else {
		return GetCephdError(int(ret))
	}
}

// Shutdown disconnects from the cluster.
func (c *conn) Shutdown() {
	C.rados_shutdown(c.cluster)
}

// ReadConfigFile configures the connection using a Ceph configuration file.
func (c *conn) ReadConfigFile(path string) error {
	c_path := C.CString(path)
	defer C.free(unsafe.Pointer(c_path))
	ret := C.rados_conf_read_file(c.cluster, c_path)
	if ret == 0 {
		return nil
	} else {
		return GetCephdError(int(ret))
	}
}

// MonCommand sends a command to one of the monitors
func (c *conn) MonCommand(args []byte) (buffer []byte, info string, err error) {
	return c.monCommand(args, nil)
}

// MonCommand sends a command to one of the monitors, with an input buffer
func (c *conn) MonCommandWithInputBuffer(args, inputBuffer []byte) (buffer []byte, info string, err error) {
	return c.monCommand(args, inputBuffer)
}

func (c *conn) monCommand(args, inputBuffer []byte) (buffer []byte, info string, err error) {
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
		err = GetCephdError(int(ret))
		return nil, info, err
	}

	return
}

// PingMonitor sends a ping to a monitor and returns the reply.
func (c *conn) PingMonitor(id string) (string, error) {
	c_id := C.CString(id)
	defer C.free(unsafe.Pointer(c_id))

	var strlen C.size_t
	var strout *C.char

	ret := C.rados_ping_monitor(c.cluster, c_id, &strout, &strlen)
	defer C.rados_buffer_free(strout)

	if ret == 0 {
		reply := C.GoStringN(strout, (C.int)(strlen))
		return reply, nil
	} else {
		return "", GetCephdError(int(ret))
	}
}

// IOContext represents a context for performing I/O within a pool.
type IOContext struct {
	ioctx C.rados_ioctx_t
}

func (c *conn) OpenIOContext(pool string) (cephclient.IOContext, error) {
	c_pool := C.CString(pool)
	defer C.free(unsafe.Pointer(c_pool))
	ioctx := &IOContext{}
	ret := C.rados_ioctx_create(c.cluster, c_pool, &ioctx.ioctx)
	if ret == 0 {
		return ioctx, nil
	} else {
		return nil, GetCephdError(int(ret))
	}
}

// Read reads up to len(data) bytes from the object with key oid starting at byte
// offset offset. It returns the number of bytes read and an error, if any.
func (ioctx *IOContext) Read(oid string, data []byte, offset uint64) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	ret := C.rados_read(
		ioctx.ioctx,
		c_oid,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)),
		(C.uint64_t)(offset))

	if ret >= 0 {
		return int(ret), nil
	} else {
		return 0, GetCephdError(int(ret))
	}
}

// Write writes len(data) bytes to the object with key oid starting at byte
// offset offset. It returns an error, if any.
func (ioctx *IOContext) Write(oid string, data []byte, offset uint64) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	ret := C.rados_write(ioctx.ioctx, c_oid,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)),
		(C.uint64_t)(offset))

	return GetCephdError(int(ret))
}

// WriteFull writes len(data) bytes to the object with key oid.
// The object is filled with the provided data. If the object exists,
// it is atomically truncated and then written. It returns an error, if any.
func (ioctx *IOContext) WriteFull(oid string, data []byte) error {
	c_oid := C.CString(oid)
	defer C.free(unsafe.Pointer(c_oid))

	ret := C.rados_write_full(ioctx.ioctx, c_oid,
		(*C.char)(unsafe.Pointer(&data[0])),
		(C.size_t)(len(data)))
	return GetCephdError(int(ret))
}
