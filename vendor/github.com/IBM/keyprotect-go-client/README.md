# IBM Cloud Go SDK Version 0.15.1

# keyprotect-go-client

[![Build Status](https://travis-ci.com/IBM/keyprotect-go-client.svg?branch=master)](https://travis-ci.com/IBM/keyprotect-go-client)
[![GoDoc](https://godoc.org/github.com/keyprotect-go-client?status.svg)](https://godoc.org/github.com/IBM/keyprotect-go-client)

keyprotect-go-client is a Go client library for interacting with the IBM KeyProtect service.

* [Questions / Support](#questions--support)
* [Usage](#usage)
  * [Migrating](#migrating)
  * [Authentication](#authentication)
  * [Finding Instance UUIDs](#finding-a-keyprotect-service-instances-uuid)
  * [Examples](#examples)
* [Contributing](/CONTRIBUTING.md)

## Questions / Support

There are many channels for asking questions about KeyProtect and this client.

- Ask a question on Stackoverflow and tag it with `key-protect` and `ibm-cloud`
- Open a [Github Issue](https://github.com/IBM/keyprotect-go-client/issues)
- If you work at IBM and have access to the internal Slack, you can join the `#key-protect` channel and ask there.

## Usage

This client expects that you have an existing IBM Cloud Key Protect Service Instance. To get started, visit the [IBM KeyProtect Catalog Page](https://cloud.ibm.com/catalog/services/key-protect).

Build a client with `ClientConfig` and `New`, then use the client to do some operations.
```go
import kp "github.com/IBM/keyprotect-go-client"

// Use your IAM API Key and your KeyProtect Service Instance GUID/UUID to create a ClientConfig
cc := kp.ClientConfig{
	BaseURL:       kp.DefaultBaseURL,
	APIKey:        "......",
	InstanceID:    "1234abcd-906d-438a-8a68-deadbeef1a2b3",
}

// Build a new client from the config
client := kp.New(cc, kp.DefaultTransport())

// List keys in your KeyProtect instance
keys, err := client.GetKeys(context.Background(), 0, 0)
```

### Migrating

For users of the original `key-protect-client` that is now deprecated, this library is a drop in replacement. Updating the package reference to `github.com/IBM/keyprotect-go-client` should be the only change needed. If you are worried about new incompatible changes, version `v0.3.1` of `key-protect-client` is equivalent to version `v0.3.3` of `keyprotect-go-client`, so pinning `v0.3.3` of the new library should be sufficient to pull from the new repo with no new functional changes.

## Authentication

The KeyProtect client requires a valid [IAM API Key](https://cloud.ibm.com/docs/iam?topic=iam-userapikey#create_user_key) that is passed via the `APIKey` field in the `ClientConfig`. The client will call IAM to get an access token for that API key, caches the access token, and reuses that token on subsequent calls. If the access token is expired, the client will call IAM to get a new access token.

Alternatively, you may also inject your own tokens during runtime. When using your own tokens, it's the responsibilty of the caller to ensure the access token is valid and is not expired. You can specify the access token in either the `ClientConfig` structure or on the context (see below.)

To specify authorization token on the context:

```go
// Create a ClientConfig and Client like before, but without an APIKey
cc := kp.ClientConfig{
	BaseURL:       kp.DefaultBaseURL,
	InstanceID:    "1234abcd-906d-438a-8a68-deadbeef1a2b3",
}
client := kp.New(cc, kp.DefaultTransport())

// Use NewContextWithAuth to add your token into the context
ctx := context.Background()
ctx = kp.NewContextWithAuth(ctx, "Bearer ABCDEF123456....")

// List keys with our injected token via the context
keys, err := api.GetKeys(ctx, 0, 0)
```

For information on IAM API Keys and tokens please refer to the [IAM docs](https://cloud.ibm.com/docs/iam?topic=iam-manapikey)

## Finding a KeyProtect Service Instance's UUID

The client requires a valid UUID that identifies your KeyProtect Service Instance to be able to interact with your key data in the instance. An instance is somewhat like a folder or directory of keys; you can have many of them per account, but the keys they contain are separate and cannot be shared between instances.

The [IBM Cloud CLI](https://cloud.ibm.com/docs/cli?topic=cloud-cli-getting-started) can be used to find the UUID for your KeyProtect instance.

```sh
$ ic resource service-instances
OK
Name                                                              Location   State    Type
Key Protect-private                                               us-south   active   service_instance
Key Protect-abc123                                                us-east    active   service_instance
```

Find the name of your KeyProtect instance as you created it, and the use the client to get its details. The Instance ID is the GUID field, or if you do not see GUID, it will be the last part of the CRN. For example:

```sh
$ ic resource service-instance "Key Protect-private"
OK

Name:                  Key Protect-private
ID:                    crn:v1:bluemix:public:kms:us-south:a/.......:1234abcd-906d-438a-8a68-deadbeef1a2b3::
GUID:                  1234abcd-906d-438a-8a68-deadbeef1a2b3
```

## Examples

### Generating a root key (CRK)

```go
// Create a root key named MyRootKey with no expiration
key, err := client.CreateRootKey(ctx, "MyRootKey", nil)
if err != nil {
    fmt.Println(err)
}
fmt.Println(key.ID, key.Name)

crkID := key.ID
```

### Generating a root key with policy overrides (CRK)

```go
enable := true
// Specify policy data
policy := kp.Policy{
  Rotation: &kp.Rotation{
    Enabled:  &enable,
    Interval: 3,
  },
  DualAuth: &kp.DualAuth{
    Enabled:  &enable,
  },
}

// Create a root key named MyRootKey with a rotation and a dualAuthDelete policy
key, err := client.CreateRootKeyWithPolicyOverrides(ctx, "MyRootKey", nil, nil, policy)
if err != nil {
    fmt.Println(err)
}
fmt.Println(key.ID, key.Name)

crkID := key.ID
```

### Wrapping and Unwrapping a DEK using a specific Root Key.

```go
myDEK := []byte{"thisisadataencryptionkey"}
// Do some encryption with myDEK
// Wrap the DEK so we can safely store it
wrappedDEK, err := client.Wrap(ctx, crkIDOrAlias, myDEK, nil)


// Unwrap the DEK
dek, err := client.Unwrap(ctx, crkIDOrAlias, wrappedDEK, nil)
// Do some encryption/decryption using the DEK
// Discard the DEK
dek = nil
```

Note you can also pass additional authentication data (AAD) to wrap and unwrap calls
to provide another level of protection for your DEK.  The AAD is a string array with
each element up to 255 chars.  For example:

```go
myAAD := []string{"First aad string", "second aad string", "third aad string"}
myDEK := []byte{"thisisadataencryptionkey"}
// Do some encryption with myDEK
// Wrap the DEK so we can safely store it
wrappedDEK, err := client.Wrap(ctx, crkIDOrAlias, myDEK, &myAAD)


// Unwrap the DEK
dek, err := client.Unwrap(ctx, crkIDOrAlias, wrappedDEK, &myAAD)
// Do some encryption/decryption using the DEK
// Discard the DEK
dek = nil
```

Have key protect create a DEK for you:
* To Get the **keyversion** along with DEK and wrapped DEK use **WrapCreateDEKV2()**

```go
dek, wrappedDek, err := client.WrapCreateDEK(ctx, crkIDOrAlias, nil)
// Do some encrypt/decrypt with the dek
// Discard the DEK
dek = nil

// Save the wrapped DEK for later.  Use Unwrap to use it.
```

Can also specify AAD:

```go
myAAD := []string{"First aad string", "second aad string", "third aad string"}
dek, wrappedDek, err := client.WrapCreateDEK(ctx, crkIDOrAlias, &myAAD)
// Do some encrypt/decrypt with the dek
// Discard the DEK
dek = nil

// Save the wrapped DEK for later.  Call Unwrap to use it, make
// sure to specify the same AAD.
```

### Fetching keys based on query parameters

```go

limit := uint32(5)
offset := uint32(0)
extractable := false
keyStates := []kp.KeyState{kp.KeyState(kp.Active), kp.KeyState(kp.Suspended)}
searchStr := "foobar"
searchQuery, _ := kp.GetKeySearchQuery(&searchStr, kp.ApplyNot(), kp.AddAliasScope())

listKeysOptions := &kp.ListKeysOptions{
  Limit : &limit,
  Offset : &offset,
  Extractable : &extractable,
  State : keyStates,
  Search: searchQuery,
}

keys, err := client.ListKeys(ctx, listKeysOptions)
if err != nil {
    fmt.Println(err)
}
fmt.Println(keys)
```

### Fetching keys in ascending or descending sorted order of parameters

```go
srtStr, _ := kp.GetKeySortStr(kp.WithCreationDate(), WithImportedDesc())

listKeysOptions := &kp.ListKeysOptions{
  Sort:srtStr,
}

keys, err := client.ListKeys(ctx, listKeysOptions)
if err != nil {
    fmt.Println(err)
}
fmt.Println(keys)
```
For more information about KeySearch visit: https://cloud.ibm.com/apidocs/key-protect#kp-get-key-search-api

### Fetching key versions based on query parameters

```go

limit := uint32(2)
offset := uint32(0)
totalCount := true

listkeyVersionsOptions := &kp.ListKeyVersionsOptions{
  Limit : &limit,
  Offset : &offset,
  TotalCount : &totalCount,
}

keyVersions, err := client.ListKeyVersions(ctx, "key_id_or_alias", listkeyVersionsOptions)
if err != nil {
    fmt.Println(err)
}
fmt.Println(keyVersions)
```

### Enable instance rotation policy

```go

intervalMonth := 3
enable := true

err := client.SetRotationInstancePolicy(context.Background(), enable, &intervalMonth)
if err != nil {
    fmt.Println(err)
}

rotationInstancePolicy, err := client.GetRotationInstancePolicy(context.Background())
if err != nil {
  fmt.Println(err)
}
fmt.Println(rotationInstancePolicy)
```

### Set key rotation policy

```go

rotationInterval := 3
enabled := true
keyRotationPolicy, err := client.SetRotationPolicy(context.Background(), "key_id_or_alias", rotationInterval, enabled)
if err != nil {
  fmt.Println(err)
}
fmt.Println(keyRotationPolicy)
```

### Enable key rotation policy

```go

keyRotationPolicy, err := client.EnableRotationPolicy(context.Background(), "key_id_or_alias")
if err != nil {
  fmt.Println(err)
}
fmt.Println(keyRotationPolicy)
```

### List keys based on filter properties

```go
// Option-1 - Directly passing the filter query in the format that the API supports.
filterQuery := "creationDate=gt:\"2022-07-05T00:00:00Z\" state=1,5 extractable=false"

listKeysOptions := &kp.ListKeysOptions{
  Filter:&filterQuery,
}

keys, err := client.ListKeys(ctx, listKeysOptions)
if err != nil {
    fmt.Println(err)
}
fmt.Println(keys)

// Option-2 - Using the builder provided by SDK to construct the filter query
fb := kp.GetFilterQueryBuilder()
dateQ := time.Date(2022, 07, 04, 07, 43, 23, 100, time.UTC)

filterQuery := fb.CreationDate().GreaterThan(dateQ).
	State([]kp.KeyState{kp.KeyState(kp.Destroyed), kp.KeyState(kp.Active)}).
	Extractable(true).
	Build()

listKeysOptions := &kp.ListKeysOptions{
  Filter:&filterQuery,
}

keys, err := client.ListKeys(ctx, listKeysOptions)
if err != nil {
    fmt.Println(err)
}
fmt.Println(keys)
```

### Support for Adding Custom Header


1) From ServiceClient (For Every API Call)
```go
cc := kp.ClientConfig{
    BaseURL:    "BASE_URL",
    APIKey:     "API_KEY",
    InstanceID: "INSTANCE_ID",
    Headers: http.Header{
      "Custom-Header":  {"Custom-Value"},
    },
  }
```

2) From ServiceCall (Per API Call)

* Define Header just before the API Call and Empty out when done.

```go
client.Config.Headers = make(http.Header))
client.Config.Headers.Set("Custom-Header", "Custom-Header-Value")

key, err := client.CreateKey(params)
  if err != nil {
    panic(err)
  }

client.Config.Headers = http.Header{}
```