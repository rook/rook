package client

type Image interface {
	Open(args ...interface{}) error
	Close() error
	Stat() (info *ImageInfo, err error)
	Name() string
}

type ImageInfo struct {
	Size              uint64
	Obj_size          uint64
	Num_objs          uint64
	Order             int
	Block_name_prefix string
	Parent_pool       int64
	Parent_name       string
}
