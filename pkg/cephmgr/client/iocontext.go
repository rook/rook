package client

// interface for the ceph io context
type IOContext interface {
	Read(oid string, data []byte, offset uint64) (int, error)
	Write(oid string, data []byte, offset uint64) error
	WriteFull(oid string, data []byte) error
	Pointer() uintptr
	GetImage(name string) Image
	GetImageNames() (names []string, err error)
	CreateImage(name string, size uint64, order int, args ...uint64) (image Image, err error)
}
