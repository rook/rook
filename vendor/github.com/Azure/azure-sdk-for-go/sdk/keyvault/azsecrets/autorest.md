## Go

These settings apply only when `--go` is specified on the command line.

```yaml
clear-output-folder: false
export-clients: true
go: true
input-file: https://github.com/Azure/azure-rest-api-specs/blob/551275acb80e1f8b39036b79dfc35a8f63b601a7/specification/keyvault/data-plane/Microsoft.KeyVault/stable/7.4/secrets.json
license-header: MICROSOFT_MIT_NO_VERSION
module: github.com/Azure/azure-sdk-for-go/sdk/keyvault/azsecrets
openapi-type: "data-plane"
output-folder: ../azsecrets
override-client-name: Client
security: "AADToken"
security-scopes: "https://vault.azure.net/.default"
use: "@autorest/go@4.0.0-preview.46"
version: "^3.0.0"

directive:
  # delete unused model
  - remove-model: SecretProperties

  # make vault URL a parameter of the client constructor
  - from: swagger-document
    where: $["x-ms-parameterized-host"]
    transform: $.parameters[0]["x-ms-parameter-location"] = "client"

  # rename parameter models to match their methods
  - rename-model:
      from: SecretRestoreParameters
      to: RestoreSecretParameters
  - rename-model:
      from: SecretSetParameters
      to: SetSecretParameters
  - rename-model:
      from: SecretUpdateParameters
      to: UpdateSecretParameters

  # rename paged operations from Get* to List*
  - rename-operation:
      from: GetDeletedSecrets
      to: ListDeletedSecrets
  - rename-operation:
      from: GetSecrets
      to: ListSecrets
  - rename-operation:
      from: GetSecretVersions
      to: ListSecretVersions

  # delete unused error models
  - from: models.go
    where: $
    transform: return $.replace(/(?:\/\/.*\s)+type (?:Error|KeyVaultError).+\{(?:\s.+\s)+\}\s/g, "");
  - from: models_serde.go
    where: $
    transform: return $.replace(/(?:\/\/.*\s)+func \(\w \*?(?:Error|KeyVaultError)\).*\{\s(?:.+\s)+\}\s/g, "");

  # delete the Attributes model defined in common.json (it's used only with allOf)
  - from: models.go
    where: $
    transform: return $.replace(/(?:\/\/.*\s)+type Attributes.+\{(?:\s.+\s)+\}\s/g, "");
  - from: models_serde.go
    where: $
    transform: return $.replace(/(?:\/\/.*\s)+func \(a \*?Attributes\).*\{\s(?:.+\s)+\}\s/g, "");

  # delete the version path param check (version == "" is legal for Key Vault but indescribable by OpenAPI)
  - from: client.go
    where: $
    transform: return $.replace(/\sif secretVersion == "" \{\s+.+secretVersion cannot be empty"\)\s+\}\s/g, "");

  # delete client name prefix from method options and response types
  - from:
      - client.go
      - models.go
      - response_types.go
    where: $
    transform: return $.replace(/Client(\w+)((?:Options|Response))/g, "$1$2");

  # make secret IDs a convenience type so we can add parsing methods
  - from: models.go
    where: $
    transform: return $.replace(/(\sID \*)string(\s+.*)/g, "$1ID$2")

  # Maxresults -> MaxResults
  - from:
      - client.go
      - models.go
    where: $
    transform: return $.replace(/Maxresults/g, "MaxResults")

  # secretName, secretVersion -> name, version
  - from: client.go
  - where: $
  - transform: return $.replace(/secretName/g, "name").replace(/secretVersion/g, "version")
```
