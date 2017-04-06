package contracts

type Irook_filesystem interface {
	FS_Create(name string) error
	FS_Delete(name string) error
	FS_List() error
	FS_Mount(name string, mountpath string) error
	FS_Write(name string,mountpath string,data string,filename string) (string,error)
	FS_Read(name string,mountpath string,filename string) (string,error)
	FS_Unmount(mountpath string) error
}
