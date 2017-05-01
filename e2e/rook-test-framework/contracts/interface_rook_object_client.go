package contracts

type Irook_object interface {
	Object_Create() (string, error)
	Object_Bucket_list() (string, error)
	Object_Connection(uerid string) (string, error)
	Object_Create_user(userid string,displayname string) (string, error)
	Object_Delete_user(userid string) (string, error)
	Object_Get_user(userid string) (string, error)
	Object_List_user() (string, error)
	Object_Update_user(userid string,displayname string,emailid string) (string, error)
}
