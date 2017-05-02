package contracts

type IRookClient interface {
	Status() (string, error)
	Version() (string, error)
	Node() (string, error)
}
