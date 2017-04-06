package contracts

type Irook_pool interface {
	Pool_List() (string, error)
	Pool_Create() (string, error)
}
