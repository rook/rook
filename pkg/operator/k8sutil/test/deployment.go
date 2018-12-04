package test

import (
	"github.com/rook/rook/pkg/clusterd"
	extensions "k8s.io/api/extensions/v1beta1"
)

// UpdateDeploymentAndWaitStub returns a stub replacement for the UpdateDeploymentAndWait function
// for unit tests which always returns success (nil). The generated simple clientset doesn't seem to
// handle the Deployment.Update method as expected. The deployment is instead zero-ed out when the
// deployment is updated with an unchanged version, which breaks unit tests.
// In order to still test the UpdateDeploymentAndWait function, the stub function returned will
// append a copy of the deployment used as input to the list of deployments updated. The function
// returns a pointer to this slice which the calling func may use to verify the expected contents of
// deploymentsUpdated based on expected behavior.
func UpdateDeploymentAndWaitStub() (
	stubFunc func(context *clusterd.Context, deployment *extensions.Deployment, namespace string) error,
	deploymentsUpdated *[]*extensions.Deployment,
) {
	deploymentsUpdated = &[]*extensions.Deployment{}
	stubFunc = func(context *clusterd.Context, deployment *extensions.Deployment, namespace string) error {
		*deploymentsUpdated = append(*deploymentsUpdated, deployment)
		return nil
	}
	return stubFunc, deploymentsUpdated
}

// DeploymentNamesUpdated converts a deploymentsUpdated slice into a string slice of deployment names
func DeploymentNamesUpdated(deploymentsUpdated *[]*extensions.Deployment) []string {
	ns := []string{}
	for _, d := range *deploymentsUpdated {
		ns = append(ns, d.GetName())
	}
	return ns
}

// ClearDeploymentsUpdated clears the deploymentsUpdated list
func ClearDeploymentsUpdated(deploymentsUpdated *[]*extensions.Deployment) {
	*deploymentsUpdated = []*extensions.Deployment{}
}
