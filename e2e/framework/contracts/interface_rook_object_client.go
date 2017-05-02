package contracts

type IRookObject interface {
	ObjectCreate() (string, error)
	ObjectBucketList() (string, error)
	ObjectConnection(uerid string) (string, error)
	ObjectCreateUser(userid string, displayname string) (string, error)
	ObjectDeleteUser(userid string) (string, error)
	ObjectGetUser(userid string) (string, error)
	ObjectListUser() (string, error)
	ObjectUpdateUser(userid string, displayname string, emailid string) (string, error)
}
