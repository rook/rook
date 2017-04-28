package contracts

type IRookFilesystem interface {
	FSCreate(name string) (string, error)
	FSDelete(name string) (string, error)
	FSList() (string, error)
	FSMount(name string, mountpath string) (string, error)
	FSWrite(name string, mountpath string, data string, filename string, namespace string) (string, error)
	FSRead(name string, mountpath string, filename string, namespace string) (string, error)
	FSUnmount(mountpath string) (string, error)
}
