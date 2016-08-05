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

import (
	"fmt"
	"unsafe"
)

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

// Mon runs embedded ceph-mon
func Mon(args []string) error {
	var cptr *C.char
	ptrSize := unsafe.Sizeof(cptr)

	// Allocate the char** list.
	ptr := C.malloc(C.size_t(len(args)) * C.size_t(ptrSize))
	defer C.free(ptr)

	// Assign each byte slice to its appropriate offset.
	for i := 0; i < len(args); i++ {
		element := (**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*ptrSize))
		*element = C.CString(args[i])
		defer C.free(unsafe.Pointer(*element))
	}

	ret := C.cephd_mon(C.int(len(args)), (**C.char)(ptr))
	if ret < 0 {
		return cephdError(int(ret))
	}

	return nil
}
