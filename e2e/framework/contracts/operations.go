package contracts

import "github.com/rook/rook/pkg/model"

type BlockOperator interface {
	BlockCreate(name string, size int) (string, error)
	BlockDelete(name string) (string, error)
	BlockList() ([]model.BlockImage, error)
	BlockMap(name string, mountpath string) (string, error)
	BlockWrite(name string, mountpath string, data string, filename string, namespace string) (string, error)
	BlockRead(name string, mountpath string, filename string, namespace string) (string, error)
	BlockUnmap(name string, mountpath string) (string, error)
}

type FileSystemOperator interface {
	FSCreate(name string) (string, error)
	FSDelete(name string) (string, error)
	FSList() ([]model.Filesystem, error)
	FSMount(name string, mountpath string) (string, error)
	FSWrite(name string, mountpath string, data string, filename string, namespace string) (string, error)
	FSRead(name string, mountpath string, filename string, namespace string) (string, error)
	FSUnmount(mountpath string) (string, error)
}

type ObjectOperator interface {
	ObjectCreate() (string, error)
	ObjectBucketList() ([]model.ObjectBucket, error)
	ObjectConnection() (*model.ObjectStoreConnectInfo, error)
	ObjectCreateUser(userid string, displayname string) (*model.ObjectUser, error)
	ObjectUpdateUser(userid string, displayname string, emailid string) (*model.ObjectUser, error)
	ObjectDeleteUser(userid string) error
	ObjectGetUser(userid string) (*model.ObjectUser, error)
	ObjectListUser() ([]model.ObjectUser, error)
}

type PoolOperator interface {
	PoolList() ([]model.Pool, error)
	PoolCreate(pool model.Pool) (string, error)
}

type RestAPIOperator interface {
	URL() string
	GetNodes() ([]model.Node, error)
	GetPools() ([]model.Pool, error)
	CreatePool(pool model.Pool) (string, error)
	GetBlockImages() ([]model.BlockImage, error)
	CreateBlockImage(image model.BlockImage) (string, error)
	DeleteBlockImage(image model.BlockImage) (string, error)
	GetClientAccessInfo() (model.ClientAccessInfo, error)
	GetFilesystems() ([]model.Filesystem, error)
	CreateFilesystem(fsmodel model.FilesystemRequest) (string, error)
	DeleteFilesystem(fsmodel model.FilesystemRequest) (string, error)
	GetStatusDetails() (model.StatusDetails, error)
	CreateObjectStore() (string, error)
	GetObjectStoreConnectionInfo() (*model.ObjectStoreConnectInfo, error)
	ListBuckets() ([]model.ObjectBucket, error)
	ListObjectUsers() ([]model.ObjectUser, error)
	GetObjectUser(id string) (*model.ObjectUser, error)
	CreateObjectUser(user model.ObjectUser) (*model.ObjectUser, error)
	UpdateObjectUser(user model.ObjectUser) (*model.ObjectUser, error)
	DeleteObjectUser(id string) error
}
