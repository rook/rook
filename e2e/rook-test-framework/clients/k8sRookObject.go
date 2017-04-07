package clients

import (
	"errors"
	"github.com/dangula/rook/e2e/rook-test-framework/contracts"
)

type k8sRookObject struct {
	transportClient contracts.ITransportClient
}

func CreateK8sRookObject(client contracts.ITransportClient) *k8sRookObject {
	return &k8sRookObject{transportClient: client}
}

func (ro *k8sRookObject) Object_Create() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_Bucket_list() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_Connection() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_Create_user() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_Delete_user() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_Get_user() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_List_user() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_Update_user() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_PutData() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
func (ro *k8sRookObject) Object_GetData() (string, error) {
	//TODO - implement
	return "Not YET IMPLEMENTED", errors.New("NOT YET IMPLEMENTED")
}
