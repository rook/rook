package contracts

type IRookPool interface {
	PoolList() (string, error)
	PoolCreate() (string, error)
}
