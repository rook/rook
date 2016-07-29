package cephd

// #cgo CFLAGS: -I${SRCDIR}/../../ceph/src/include
// #cgo LDFLAGS: -L${SRCDIR}/../../ceph/build/lib -lcephd -lstdc++ -lm -ldl -lboost_system -lboost_thread -lboost_iostreams -lz -lcrypto++
// #include <errno.h>
// #include <stdlib.h>
// #include <string.h>
// #include "cephd/libcephd.h"
import "C"

import (
	"fmt"
	"unsafe"
)

type CephdError int

func (e CephdError) Error() string {
	return fmt.Sprintf("cephd: %s", C.GoString(C.strerror(C.int(-e))))
}

// Conn is a connection handle to a Ceph cluster.
type Context struct {
	context C.cephd_t
}

func Version() string {
	var cMajor, cMinor, cPatch C.int
	return C.GoString(C.ceph_version(&cMajor, &cMinor, &cPatch))
}

func NewContext() (*Context, error) {
	cxt := &Context{}
	ret := C.cephd_create(&cxt.context)

	if ret == 0 {
		return cxt, nil
	}

	return nil, CephdError(int(ret))
}

func (c *Context) CreateKey() (key string, err error) {
	buf := make([]byte, 128)
	ret := int(C.cephd_create_key(c.context,
		(*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf))))
	// FIXME: ret may be -ERANGE if the buffer is not large enough.
	if ret >= 0 {
		key = C.GoString((*C.char)(unsafe.Pointer(&buf[0])))
		return key, nil
	}

	return "", CephdError(ret)
}
