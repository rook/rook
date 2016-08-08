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
