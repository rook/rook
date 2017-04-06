package contracts

type Irook_block interface {
	Block_Create(name string, size int) (string, error)
	Block_Delete(name string) (string, error)
	Block_List() (string, error)
	Block_Map(name string, mountpath string) (string, error)
	Block_Write(name string,mountpath string,data string,filename string) (string,error)
	Block_Read(name string,mountpath string,filename string) (string,error)
	Block_Unmap(name string, mountpath string) (string, error)

}
