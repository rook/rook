package main

type Utility interface {
	Exec(input []byte, name string, args ...string) ([]byte, []byte, error)
	Kubectl(ns string, args ...string) ([]byte, []byte, error)
	Ceph(ns string, args ...string) ([]byte, []byte, error)
	AskUser(message string) error
}

type UtilityImpl struct {
}
