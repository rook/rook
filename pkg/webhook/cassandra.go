package webhook

import (
	"fmt"
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/client/clientset/versioned"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"reflect"
)

type CassandraAdmission struct {
	rookClient *versioned.Interface
}

func (cassadm *CassandraAdmission) Validate(req *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {

	logger.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo)

	allowed, msg := func() (allowed bool, msg string) {
		// Extract object from the AdmissionRequest
		old, new := &cassandrav1alpha1.Cluster{}, &cassandrav1alpha1.Cluster{}
		if err := unmarshalObjects(req, old, new); err != nil {
			return false, err.Error()
		}
		allowed, msg = cassadm.checkValues(new)
		if allowed && old != nil {
			allowed, msg = cassadm.checkTransitions(old, new)
		}
		return
	}()

	return &admissionv1beta1.AdmissionResponse{
		Allowed: allowed,
		Result: &metav1.Status{
			Message: msg,
		},
	}
}

// checkValues checks that the values are valid
func (cassadm *CassandraAdmission) checkValues(c *cassandrav1alpha1.Cluster) (allowed bool, msg string) {

	rackNames := sets.NewString()
	for _, rack := range c.Spec.Datacenter.Racks {
		// Check that no two racks have the same name
		if rackNames.Has(rack.Name) {
			return false, fmt.Sprintf("two racks have the same name: '%s'", rack.Name)
		}
		rackNames.Insert(rack.Name)

		// Check that persistent storage is configured
		if rack.Storage.VolumeClaimTemplates == nil {
			return false, fmt.Sprintf("rack '%s' has no volumeClaimTemplates defined", rack.Name)
		}

		// Check that only one disk is present
		if len(rack.Storage.VolumeClaimTemplates) > 1 {
			return false, fmt.Sprintf("rack '%s' has more than one volumeClaimTemplates, currently only 1 is supported", rack.Name)
		}

		// Check that configMapName is not set
		if rack.ConfigMapName != nil {
			return false, fmt.Sprintf("rack '%s' has configMapName set which is currently not supported", rack.Name)
		}

	}

	return true, ""
}

// checkTransitions checks that the new values are valid given the old values of the object
func (cassadm *CassandraAdmission) checkTransitions(old, new *cassandrav1alpha1.Cluster) (allowed bool, msg string) {

	// Check that version remained the same
	if old.Spec.Version != new.Spec.Version {
		return false, "change of version is currently not supported"
	}

	// Check that repository remained the same
	if old.Spec.Repository != new.Spec.Repository {
		return false, "repository change is currently not supported"
	}

	// Check that mode remained the same
	if old.Spec.Mode != new.Spec.Mode {
		return false, "change of mode is currently not supported"
	}

	// Check that sidecarImage remained the same
	if !reflect.DeepEqual(old.Spec.SidecarImage, new.Spec.SidecarImage) {
		return false, "change of sidecarImage is currently not supported"
	}

	// Check that the datacenter name didn't change
	if old.Spec.Datacenter.Name != new.Spec.Datacenter.Name {
		return false, "change of datacenter name is currently not supported"
	}

	// Check that all rack names are the same as before
	oldRackNames, newRackNames := sets.NewString(), sets.NewString()
	for _, rack := range old.Spec.Datacenter.Racks {
		oldRackNames.Insert(rack.Name)
	}
	for _, rack := range new.Spec.Datacenter.Racks {
		newRackNames.Insert(rack.Name)
	}
	diff := oldRackNames.Difference(newRackNames)
	if diff.Len() != 0 {
		return false, fmt.Sprintf("racks %v not found, you cannot remove racks from the spec", diff.List())
	}

	rackMap := make(map[string]cassandrav1alpha1.RackSpec)
	for _, oldRack := range old.Spec.Datacenter.Racks {
		rackMap[oldRack.Name] = oldRack
	}
	for _, newRack := range new.Spec.Datacenter.Racks {
		oldRack := rackMap[newRack.Name]

		// Check that placement is the same as before
		if !reflect.DeepEqual(oldRack.Placement, newRack.Placement) {
			return false, fmt.Sprintf("rack %s: changes in placement are not currently supported", oldRack.Name)
		}

		// Check that storage is the same as before
		if !reflect.DeepEqual(oldRack.Storage, newRack.Storage) {
			return false, fmt.Sprintf("rack %s: changes in storage are not currently supported", oldRack.Name)
		}

		// Check that resources are the same as before
		if !reflect.DeepEqual(oldRack.Resources, newRack.Resources) {
			return false, fmt.Sprintf("rack %s: changes in resources are not currently supported", oldRack.Name)
		}
	}

	return true, ""
}
