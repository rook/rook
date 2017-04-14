package contracts

type Irook_client interface {
	Status() (string, error)
	Version() (string, error)
	Node() (string, error)
}
