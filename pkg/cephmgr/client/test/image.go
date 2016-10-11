package test

import "github.com/quantum/castle/pkg/cephmgr/client"

type MockImage struct {
	MockName string
	MockStat func() (info *client.ImageInfo, err error)
}

func (i *MockImage) Open(args ...interface{}) error {
	return nil
}

func (i *MockImage) Close() error {
	return nil
}

func (i *MockImage) Stat() (info *client.ImageInfo, err error) {
	if i.MockStat != nil {
		return i.MockStat()
	}
	return nil, nil
}

func (i *MockImage) Name() string {
	return i.MockName
}
