package secrets

import (
	"context"
	"fmt"
	"sync"
)

type ReaderInit func(map[string]interface{}) (SecretReader, error)
type StoreInit func(map[string]interface{}) (SecretStore, error)

var (
	secretReaders = make(map[string]ReaderInit)
	secretStores  = make(map[string]StoreInit)
	readersLock   sync.RWMutex
	storesLock    sync.RWMutex
)

// NewReader returns a new instance of SecretReader backend SM identified by
// the supplied name. SecretConfig is a map of key value pairs which could
// be used for authenticating with the backend
func NewReader(name string, secretConfig map[string]interface{}) (SecretReader, error) {
	readersLock.RLock()
	defer readersLock.RUnlock()

	if init, exists := secretReaders[name]; exists {
		return init(secretConfig)
	}
	return nil, ErrNotSupported
}

// NewStore returns a new instance of SecretStore backend SM identified by
// the supplied name. SecretConfig is a map of key value pairs which could
// be used for authenticating with the backend
func NewStore(name string, secretConfig map[string]interface{}) (SecretStore, error) {
	storesLock.RLock()
	defer storesLock.RUnlock()

	if init, exists := secretStores[name]; exists {
		return init(secretConfig)
	}
	return nil, ErrNotSupported
}

// RegisterReader adds a new backend KMS that implements SecretReader
func RegisterReader(name string, init ReaderInit) error {
	readersLock.Lock()
	defer readersLock.Unlock()

	if _, exists := secretReaders[name]; exists {
		return fmt.Errorf("secrets reader %v is already registered", name)
	}
	secretReaders[name] = init
	return nil
}

// RegisterStore adds a new backend KMS that implements SecretStore and SecretReader
func RegisterStore(name string, init StoreInit) error {
	storesLock.Lock()
	defer storesLock.Unlock()

	if _, exists := secretStores[name]; exists {
		return fmt.Errorf("secrets store %v is already registered", name)
	}
	secretStores[name] = init

	return RegisterReader(name, func(m map[string]interface{}) (SecretReader, error) {
		return init(m)
	})
}

// A SecretKey identifies a secret
type SecretKey struct {
	// Prefix is an optional part of the SecretKey.
	Prefix string
	// Name is a mandatory part of the SecretKey.
	Name string
}

// SecretReader interface implemented by Secrets Managers to read secrets
type SecretReader interface {
	// String representation of the backend.
	String() string
	// Get returns the secret associate with the supplied key.
	Get(ctx context.Context, key SecretKey) (secret map[string]interface{}, err error)
}

// SecretStore interface implemented by Secrets Managers to set and delete secrets.
type SecretStore interface {
	SecretReader
	// Set stores the secret data identified by the key.
	// The caller should ensure they use unique key so that they won't
	// unknowingly overwrite an existing secret.
	Set(ctx context.Context, key SecretKey, secret map[string]interface{}) error
	// Delete deletes the secret data associated with the supplied key.
	Delete(ctx context.Context, key SecretKey) error
}
