package vault

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/hashicorp/vault/api"
	"github.com/libopenstorage/secrets"
	"github.com/libopenstorage/secrets/vault/utils"
	"github.com/sirupsen/logrus"
)

const (
	Name                = secrets.TypeVault
	DefaultBackendPath  = "secret/"
	VaultBackendPathKey = "VAULT_BACKEND_PATH"
	VaultBackendKey     = "VAULT_BACKEND"
	kvVersionKey        = "version"
	kvDataKey           = "data"
	kvMetadataKey       = "metadata"
	kvVersion1          = "kv"
	kvVersion2          = "kv-v2"

	AuthMethodKubernetes    = utils.AuthMethodKubernetes
	AuthMethod              = utils.AuthMethod
	AuthMountPath           = utils.AuthMountPath
	AuthKubernetesRole      = utils.AuthKubernetesRole
	AuthKubernetesTokenPath = utils.AuthKubernetesTokenPath
	AuthKubernetesMountPath = utils.AuthKubernetesMountPath
	AuthAppRoleRoleID       = utils.AuthAppRoleRoleID
	AuthAppRoleSecretID     = utils.AuthAppRoleSecretID
)

func init() {
	if err := secrets.Register(Name, New); err != nil {
		panic(err.Error())
	}
}

type vaultSecrets struct {
	mu     sync.RWMutex
	client *api.Client

	currentNamespace string
	lockClientToken  sync.Mutex

	endpoint      string
	backendPath   string
	namespace     string
	isKvBackendV2 bool
	autoAuth      bool
	config        map[string]interface{}
}

// These variables are helpful in testing to stub method call from packages
var (
	newVaultClient = api.NewClient
	isKvV2         = isKvBackendV2
)

func New(
	secretConfig map[string]interface{},
) (secrets.Secrets, error) {
	// DefaultConfig uses the environment variables if present.
	config := api.DefaultConfig()

	if len(secretConfig) == 0 && config.Error != nil {
		return nil, config.Error
	}

	address := utils.GetVaultParam(secretConfig, api.EnvVaultAddress)
	if address == "" {
		return nil, utils.ErrVaultAddressNotSet
	}
	if err := utils.IsValidAddr(address); err != nil {
		return nil, err
	}
	config.Address = address

	if err := utils.ConfigureTLS(config, secretConfig); err != nil {
		return nil, err
	}

	client, err := newVaultClient(config)
	if err != nil {
		return nil, err
	}

	namespace := utils.GetVaultParam(secretConfig, api.EnvVaultNamespace)
	if len(namespace) > 0 {
		// use a namespace as a header for setup purposes
		// later use it as a key prefix
		client.SetNamespace(namespace)
		defer client.SetNamespace("")
	}

	token, autoAuth, err := utils.Authenticate(client, secretConfig)
	if err != nil {
		utils.CloseIdleConnections(config)
		return nil, fmt.Errorf("failed to get the authentication token: %w", err)
	}
	client.SetToken(token)

	authMethod := "token"
	method := utils.GetVaultParam(secretConfig, AuthMethod)
	if method != "" && utils.GetVaultParam(secretConfig, api.EnvVaultToken) == "" {
		authMethod = method
	}

	logrus.Infof("Authenticated to Vault with %v\n", authMethod)

	backendPath := utils.GetVaultParam(secretConfig, VaultBackendPathKey)
	if backendPath == "" {
		backendPath = DefaultBackendPath
	}

	var isBackendV2 bool
	backend := utils.GetVaultParam(secretConfig, VaultBackendKey)
	if backend == kvVersion1 {
		isBackendV2 = false
	} else if backend == kvVersion2 {
		isBackendV2 = true
	} else {
		// TODO: Handle backends other than kv
		isBackendV2, err = isKvV2(client, backendPath)
		if err != nil {
			utils.CloseIdleConnections(config)
			return nil, err
		}
	}
	return &vaultSecrets{
		endpoint:         config.Address,
		namespace:        namespace,
		currentNamespace: namespace,
		client:           client,
		backendPath:      backendPath,
		isKvBackendV2:    isBackendV2,
		autoAuth:         autoAuth,
		config:           secretConfig,
	}, nil
}

func (v *vaultSecrets) String() string {
	return Name
}

func (v *vaultSecrets) keyPath(secretID string, keyContext map[string]string) keyPath {
	backendPath := v.backendPath
	var namespace string
	var ok bool
	if namespace, ok = keyContext[secrets.KeyVaultNamespace]; !ok {
		namespace = v.namespace
	}

	var isDestroyKey bool
	if v.isKvBackendV2 {
		if destroyAllSecrets, ok := keyContext[secrets.DestroySecret]; ok {
			// checking for any value seems sufficient to assume 'destroy' is requested
			if destroyAllSecrets != "" {
				isDestroyKey = true
			}
		}
	}
	return keyPath{
		backendPath:  backendPath,
		isBackendV2:  v.isKvBackendV2,
		namespace:    namespace,
		secretID:     secretID,
		isDestroyKey: isDestroyKey,
	}
}

func (v *vaultSecrets) GetSecret(
	secretID string,
	keyContext map[string]string,
) (map[string]interface{}, secrets.Version, error) {
	key := v.keyPath(secretID, keyContext)
	secretValue, err := v.read(key)
	if err != nil {
		return nil, secrets.NoVersion, fmt.Errorf("failed to get secret: %s: %s", key, err)
	}
	if secretValue == nil {
		return nil, secrets.NoVersion, secrets.ErrInvalidSecretId
	}

	if v.isKvBackendV2 {
		if data, exists := secretValue.Data[kvDataKey]; exists && data != nil {
			if data, ok := data.(map[string]interface{}); ok {
				// TODO: Vault does support versioned secrets with KV Backend 2
				// However it requires clients to invoke a different metadata API call
				// to fetch the version. Once there is a need for versions from Vault
				// we can add support for it.
				return data, secrets.NoVersion, nil
			}
		}
		return nil, secrets.NoVersion, secrets.ErrInvalidSecretId
	} else {
		return secretValue.Data, secrets.NoVersion, nil
	}
}

func (v *vaultSecrets) PutSecret(
	secretID string,
	secretData map[string]interface{},
	keyContext map[string]string,
) (secrets.Version, error) {
	if v.isKvBackendV2 {
		secretData = map[string]interface{}{
			kvDataKey: secretData,
		}
	}

	key := v.keyPath(secretID, keyContext)
	if _, err := v.write(key, secretData); err != nil {
		return secrets.NoVersion, fmt.Errorf("failed to put secret: %s: %s", key, err)
	}
	return secrets.NoVersion, nil
}

func (v *vaultSecrets) DeleteSecret(
	secretID string,
	keyContext map[string]string,
) error {
	key := v.keyPath(secretID, keyContext)
	if _, err := v.delete(key); err != nil {
		return fmt.Errorf("failed to delete secret: %s: %s", key, err)
	}
	return nil
}

func (v *vaultSecrets) Encrypt(
	secretID string,
	plaintTextData string,
	keyContext map[string]string,
) (string, error) {
	return "", secrets.ErrNotSupported
}

func (v *vaultSecrets) Decrypt(
	secretID string,
	encryptedData string,
	keyContext map[string]string,
) (string, error) {
	return "", secrets.ErrNotSupported
}

func (v *vaultSecrets) Rencrypt(
	originalSecretID string,
	newSecretID string,
	originalKeyContext map[string]string,
	newKeyContext map[string]string,
	encryptedData string,
) (string, error) {
	return "", secrets.ErrNotSupported
}

func (v *vaultSecrets) ListSecrets() ([]string, error) {
	return nil, secrets.ErrNotSupported
}

func (v *vaultSecrets) read(path keyPath) (*api.Secret, error) {
	if v.autoAuth {
		v.lockClientToken.Lock()
		defer v.lockClientToken.Unlock()

		if err := v.setNamespaceToken(path.Namespace()); err != nil {
			return nil, err
		}
	}

	secretValue, err := v.lockedRead(path.Path())
	if v.isTokenExpired(err) {
		if err = v.renewToken(path.Namespace()); err != nil {
			return nil, fmt.Errorf("failed to renew token: %s", err)
		}
		return v.lockedRead(path.Path())
	}
	return secretValue, err
}

func (v *vaultSecrets) write(path keyPath, data map[string]interface{}) (*api.Secret, error) {
	if v.autoAuth {
		v.lockClientToken.Lock()
		defer v.lockClientToken.Unlock()

		if err := v.setNamespaceToken(path.Namespace()); err != nil {
			return nil, err
		}
	}

	secretValue, err := v.lockedWrite(path.Path(), data)
	if v.isTokenExpired(err) {
		if err = v.renewToken(path.Namespace()); err != nil {
			return nil, fmt.Errorf("failed to renew token: %s", err)
		}
		return v.lockedWrite(path.Path(), data)
	}
	return secretValue, err
}

func (v *vaultSecrets) delete(path keyPath) (*api.Secret, error) {
	if v.autoAuth {
		v.lockClientToken.Lock()
		defer v.lockClientToken.Unlock()

		if err := v.setNamespaceToken(path.Namespace()); err != nil {
			return nil, err
		}
	}

	secretValue, err := v.lockedDelete(path.Path())
	if v.isTokenExpired(err) {
		if err = v.renewToken(path.Namespace()); err != nil {
			return nil, fmt.Errorf("failed to renew token: %s", err)
		}
		return v.lockedDelete(path.Path())
	}
	return secretValue, err
}

func (v *vaultSecrets) lockedRead(path string) (*api.Secret, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.client.Logical().Read(path)
}

func (v *vaultSecrets) lockedWrite(path string, data map[string]interface{}) (*api.Secret, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.client.Logical().Write(path, data)
}

func (v *vaultSecrets) lockedDelete(path string) (*api.Secret, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	return v.client.Logical().Delete(path)
}

func (v *vaultSecrets) renewToken(namespace string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if len(namespace) > 0 {
		v.client.SetNamespace(namespace)
		defer v.client.SetNamespace("")
	}
	token, err := utils.GetAuthToken(v.client, v.config)
	if err != nil {
		return fmt.Errorf("get auth token for %s namespace: %s", namespace, err)
	}

	v.currentNamespace = namespace
	v.client.SetToken(token)
	return nil
}

func (v *vaultSecrets) isTokenExpired(err error) bool {
	return err != nil && v.autoAuth && strings.Contains(err.Error(), "permission denied")
}

// setNamespaceToken  is used for a multi-token support with a kubernetes auto auth setup.
//
// This allows to talk with a multiple vault namespaces (which are not sub-namespace). Create
// the same “Kubernetes Auth Role” in each of the configured namespace. For every request it
// fetches the token for that specific namespace.
func (v *vaultSecrets) setNamespaceToken(namespace string) error {
	if v.currentNamespace == namespace {
		return nil
	}

	return v.renewToken(namespace)
}

func isKvBackendV2(client *api.Client, backendPath string) (bool, error) {
	mounts, err := client.Sys().ListMounts()
	if err != nil {
		return false, err
	}

	for path, mount := range mounts {
		// path is represented as 'path/'
		if trimSlash(path) == trimSlash(backendPath) {
			version := mount.Options[kvVersionKey]
			if version == "2" {
				return true, nil
			}
			return false, nil
		}
	}

	return false, fmt.Errorf("Secrets engine with mount path '%s' not found",
		backendPath)
}

func trimSlash(in string) string {
	return strings.Trim(in, "/")
}

type keyPath struct {
	backendPath  string
	isBackendV2  bool
	namespace    string
	secretID     string
	isDestroyKey bool
}

func (k keyPath) Path() string {
	if k.isBackendV2 {
		keyType := kvDataKey
		if k.isDestroyKey {
			keyType = kvMetadataKey
		}
		return path.Join(k.namespace, k.backendPath, keyType, k.secretID)
	}
	return path.Join(k.namespace, k.backendPath, k.secretID)
}

func (k keyPath) Namespace() string {
	return k.namespace
}

func (k keyPath) String() string {
	return fmt.Sprintf("backendPath=%s, backendV2=%t, namespace=%s, secretID=%s", k.backendPath, k.isBackendV2, k.namespace, k.secretID)
}
