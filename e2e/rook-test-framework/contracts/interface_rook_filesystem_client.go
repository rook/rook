package contracts

type Irook_filesystem interface {
	FS_Create(name string) (string, error)
	FS_Delete(name string) (string, error)
	FS_List() (string, error)
	FS_Mount(name string, mountpath string) (string, error)
	FS_Write(name string,mountpath string,data string,filename string) (string,error)
	FS_Read(name string,mountpath string,filename string) (string,error)
	FS_Unmount(mountpath string) (string,error)
}
