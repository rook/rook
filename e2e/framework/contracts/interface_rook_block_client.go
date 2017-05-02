package contracts

type IRookBlock interface {
	BlockCreate(name string, size int) (string, error)
	BlockDelete(name string) (string, error)
	BlockList() (string, error)
	BlockMap(name string, mountpath string) (string, error)
	BlockWrite(name string, mountpath string, data string, filename string, namespace string) (string, error)
	BlockRead(name string, mountpath string, filename string, namespace string) (string, error)
	BlockUnmap(name string, mountpath string) (string, error)
}
