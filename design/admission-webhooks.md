# Admission Webhooks 

## Introduction

From the Kubernetes documentation:

>An admission controller is a piece of code that intercepts requests to the Kubernetes API server prior to persistence of the object, but after the request is authenticated and authorized. […] Admission controllers may be “validating”, “mutating”, or both. Mutating controllers may modify the objects they admit; validating controllers may not. […] If any of the controllers in either phase reject the request, the entire request is rejected immediately and an error is returned to the end-user.


Kubernetes lets us register specific webhooks (basically HTTPS servers) that intercept the flow of a user request to the API server. 

## Validating Webhook

### Why is it useful

A validating webhook checks if a request (CREATE, UPDATE) brings an object to a forbidden state and if it does, rejects the request. Using it together with OpenAPI validation can dramatically improve the UX of an operator and catch mistakes before they happen.

**User Stories:**

* George changes the version of Cassandra from 3.2 to 3.1. Cassandra operator doesn't support downgrades, so when George tries to apply his changes, he will get an error explaining the problem.
* Zoe tries to create a Cassandra cluster, in which two racks have the same name. Zoe gets an error when applying the new manifest, explaining that two rack can't have the same name.

### Example Implementation

To help storage providers easily write their own validating webhook, we examine the implementation for Cassandra:

1. We create the file where our webhook implementation will live, `pkg/webhook/cassandra.go`. To register a new validating webhook, we just need to implement the `AdmissionFns` interface:
```go
// pkg/webhook/webhook.go

type AdmissionFns interface {
	Validate(*admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse
}
```

```go
// pkg/webhook/cassandra.go

// Our new struct that will implement AdmissionFns
type CassandraAdmission struct {}

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
```

The `Validate` function is pretty standard and you can copy most of it. It extracts the `old` and `new` Cluster Objects and calls `checkValues` and `checkTransitions` to check that the values and the transition of the values is valid. `checkValues` and `checkTransitions` is where the logic for how to validate your CRD goes.

```go 
// checkTransitions checks that the new values are valid given the old values of the object
func (cassadm *CassandraAdmission) checkTransitions(old, new *cassandrav1alpha1.Cluster) (allowed bool, msg string) {

	// Check that version remained the same
	if old.Spec.Version != new.Spec.Version {
		return false, "change of version is currently not supported"
	}
}
	
// checkValues checks that the values are valid
func (cassadm *CassandraAdmission) checkValues(c *cassandrav1alpha1.Cluster) (allowed bool, msg string) {

    for _, rack := range c.Spec.Datacenter.Racks {
        // Check that persistent storage is configured
        if rack.Storage.VolumeClaimTemplates == nil {
        	return false, fmt.Sprintf("rack '%s' has no volumeClaimTemplates defined", rack.Name)
        }
    }
}

```

2. After this is done, we just need to write a new command to start our webhook. In our case, the command will be `rook cassandra webhook` and will live in `cmd/rook/cassandra/webhook.go`:

```go
// cmd/rook/cassandra/webhook.go 

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Runs the cassandra operator to deploy and manage cassandra in Kubernetes",
	Long: `Runs the cassandra operator to deploy and manage cassandra in kubernetes clusters.
https://github.com/rook/rook`,
}

var whConfig webhook.WebhookConfig

func init() {
	flags.SetFlagsFromEnv(webhookCmd.Flags(), rook.RookEnvVarPrefix)
	webhookCmd.Flags().Int32Var(&whConfig.Port, "port", 443, "Webhook server port.")
	webhookCmd.Flags().StringVar(&whConfig.TLSCertFile, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	webhookCmd.Flags().StringVar(&whConfig.TLSKeyFile, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tlsCertFile.")

	webhookCmd.RunE = startAdmissionWebhook
}

func startAdmissionWebhook(cmd *cobra.Command, args []string) error {

	// Create and start webhook server
	whServer := webhook.NewServerFromConfig(whConfig, &webhook.CassandraAdmission{})
	go whServer.Run()

	logger.Info("Server started...")
	// Listen for OS shutdown signal
	stopCh := server.SetupSignalHandler()
	<-stopCh

	logger.Info("Got OS shutdown signal, shutting down gracefully...")
	whServer.Server.Shutdown(context.Background())
	return nil
}
```

3. We add the scheme for our types in the init function of `pkg/webhook/webhook.go`, so that the server knows how to deserialize our Objects:
```go
// pkg/webhook/webhook.go

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)
	// Our newly added scheme
	_ = cassandrav1alpha1.AddToScheme(runtimeScheme)
}
```

4. Finally, we add the new command to our root command in `cmd/rook/cassandra.go`:
```go
// cmd/rook/cassandra.go

func init() {
	Cmd.AddCommand(operatorCmd)
	Cmd.AddCommand(sidecarCmd)
	// Our new command
	Cmd.AddCommand(webhookCmd)
}
```

5. Coding is all done! All that remains is the yaml manifests to deploy our webhook server. An example of those manifests can be found under `/cluster/examples/kubernetes/cassandra/`.