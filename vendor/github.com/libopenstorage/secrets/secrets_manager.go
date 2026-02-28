package secrets

import (
	"fmt"
	"sync"
)

var (
	instance       Secrets
	secretBackends = make(map[string]BackendInit)
	lock           sync.RWMutex
)

// Instance returns the instance set via SetInstance. nil if not set.
func Instance() Secrets {
	return instance
}

// SetInstance sets the singleton instance of the secrets backend.
func SetInstance(secretsInstance Secrets) error {
	if secretsInstance != nil {
		lock.Lock()
		defer lock.Unlock()
		instance = secretsInstance
		return nil
	}
	return fmt.Errorf("Secrets instance cannot be nil")
}

// New returns a new instance of Secrets backend KMS identified by
// the supplied name. SecretConfig is a map of key value pairs which could
// be used for authenticating with the backend
func New(
	name string,
	secretConfig map[string]interface{},
) (Secrets, error) {
	lock.RLock()
	defer lock.RUnlock()

	if bInit, exists := secretBackends[name]; exists {
		return bInit(secretConfig)
	}
	return nil, ErrNotSupported
}

// Register adds a new backend KMS
func Register(name string, bInit BackendInit) error {
	lock.Lock()
	defer lock.Unlock()
	if _, exists := secretBackends[name]; exists {
		return fmt.Errorf("Secrets Backend provider %v is already"+
			" registered", name)
	}
	secretBackends[name] = bInit
	return nil
}
