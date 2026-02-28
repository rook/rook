package azure

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets"
)

func getAzureKVParams(secretConfig map[string]interface{}, name string) string {
	if tokenIntf, exists := secretConfig[name]; exists {
		return tokenIntf.(string)
	} else {
		return os.Getenv(name)
	}
}

func getAzureVaultClient(clientID, secretID, tenantID, vaultURL string, opts azcore.ClientOptions) (*azsecrets.Client, error) {
	cred, err := azidentity.NewClientSecretCredential(tenantID, clientID, secretID, &azidentity.ClientSecretCredentialOptions{ClientOptions: opts})
	if err != nil {
		return nil, fmt.Errorf("failed to get client secret credentials. %v", err)
	}
	client, err := azsecrets.NewClient(vaultURL, cred, &azsecrets.ClientOptions{ClientOptions: opts})
	if err != nil {
		return nil, fmt.Errorf("failed to get client to access azure kv secrets. %v", err)
	}

	return client, nil
}

func getAzureVaultClientWithCert(clientID, tenantID, vaultURL, certPath, certPassword string, opts azcore.ClientOptions) (*azsecrets.Client, error) {
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("failed read certificate from path %q. %v", certPath, err)
	}

	var passphrase []byte
	if certPassword == "" {
		passphrase = nil
	} else {
		passphrase = []byte(certPassword)
	}

	certs, key, err := azidentity.ParseCertificates(certData, passphrase)
	if err != nil {
		return nil, fmt.Errorf("failed load certificate and private key. %v", err)
	}

	cred, err := azidentity.NewClientCertificateCredential(tenantID, clientID, certs, key, &azidentity.ClientCertificateCredentialOptions{ClientOptions: opts})
	if err != nil {
		return nil, fmt.Errorf("failed to construct client certificate credentials. %v", err)
	}

	client, err := azsecrets.NewClient(vaultURL, cred, &azsecrets.ClientOptions{ClientOptions: opts})
	if err != nil {
		return nil, fmt.Errorf("failed to get client to access azure kv secrets. %v", err)
	}

	return client, nil
}
