package contracts

type Irook_object interface {
	Object_Create() error
	Object_Bucket_list() error
	Object_Connection() error
	Object_Create_user() error
	Object_Delete_user() error
	Object_Get_user() error
	Object_List_user() error
	Object_Update_user() error
	Object_PutData() error
	Object_GetData() error
}
