package contracts

type Irook_object interface {
	Object_Create() (string, error)
	Object_Bucket_list() (string, error)
	Object_Connection() (string, error)
	Object_Create_user() (string, error)
	Object_Delete_user() (string, error)
	Object_Get_user() (string, error)
	Object_List_user() (string, error)
	Object_Update_user() (string, error)
	Object_PutData() (string, error)
	Object_GetData() (string, error)
}
