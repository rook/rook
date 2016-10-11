package test

import "github.com/quantum/castle/pkg/cephmgr/client"

/////////////////////////////////////////////////////////////
// implement the interface for the ceph io context
/////////////////////////////////////////////////////////////
type MockIOContext struct {
	MockGetImageNames func() (names []string, err error)
	MockGetImage      func(name string) client.Image
	MockCreateImage   func(name string, size uint64, order int, args ...uint64) (image client.Image, err error)
}

func (m *MockIOContext) Read(oid string, data []byte, offset uint64) (int, error) {
	return 0, nil
}
func (m *MockIOContext) Write(oid string, data []byte, offset uint64) error {
	return nil
}
func (m *MockIOContext) WriteFull(oid string, data []byte) error {
	return nil
}
func (m *MockIOContext) Pointer() uintptr {
	return uintptr(0)
}
func (m *MockIOContext) GetImage(name string) client.Image {
	if m.MockGetImage != nil {
		return m.MockGetImage(name)
	}
	return nil
}
func (m *MockIOContext) GetImageNames() (names []string, err error) {
	if m.MockGetImageNames != nil {
		return m.MockGetImageNames()
	}
	return nil, nil
}
func (m *MockIOContext) CreateImage(name string, size uint64, order int, args ...uint64) (image client.Image, err error) {
	if m.MockCreateImage != nil {
		return m.MockCreateImage(name, size, order, args...)
	}
	return &MockImage{MockName: name}, nil
}
