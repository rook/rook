---
# AWS server side encryption SSE-S3 support for RGW
target-version: release-1.10
---

# Feature Name
AWS server side encryption SSE-S3 support for RGW

## Summary
The S3 protocol supports three different types of [server side encryption](https://docs.aws.amazon.com/AmazonS3/latest/userguide/serv-side-encryption.html): SSE-C, SSE-KMS and SSE-S3. For the last two RGW server need to configure with external services such as [vault](https://www.vaultproject.io/). Currently Rook configure RGW with `SSE-KMS` options to handle the S3 requests with the `sse:kms` header. Recently the support for handling the `sse:s3` was added to RGW, so Rook will now provide the option to configure RGW with `sse:s3`.

The `sse:s3` is supported only from Ceph v17 an onwards, so this feature can only be enabled for Quincy or newer.

### Goals
Configure RGW with `SSE-S3` options, so that RGW can handle request with `sse:s3` headers.

## Proposal details
Introducing new field `s3` in `SecuritySpec` which defines `AWS-SSE-S3` support for RGW.
```yaml
security:
  kms:
    ..
  s3:
    connectionDetails:
      KMS_PROVIDER: vault
      VAULT_ADDR: https://vault.default.svc.cluster.local:8200
      VAULT_SECRET_ENGINE: transit
      VAULT_AUTH_METHOD: token
      ENCRYPT_ALL_OBC: true
    # name of the k8s secret containing the kms authentication token
    tokenSecretName: rook-vault-token
```
The values for this configuration will be available in [Security field](Documentation/CRDs/Object-Storage/ceph-object-store-crd.md#security-settings) of `CephObjectStoreSpec`, so depending on the above option RGW can be configured with `SSE-KMS` and `SSE-S3` options. These two options can be configured independently and they both are mutually exclusive in Rook and Ceph level.

The `ENCRYPT_ALL_OBC` will set [PutBucketEncryption](https://docs.aws.amazon.com/AmazonS3/latest/userguide/default-bucket-encryption.html) on all the bucket provisioned via obc by the Rook. So the all objects in that obc will encyrpt using `SSE-S3`. This can done manually via
s3 api using following:
```
aws s3api put-bucket-encryption --no-verify-ssl --endpoint-url http://$BUCKET_HOST:$BUCKET_PORT --bucket $BUCKET_NAME  --server-side-encryption-configuration '{
    "Rules": [
        {
            "ApplyServerSideEncryptionByDefault": {
                "SSEAlgorithm": "AES256"
            }
        }
    ]
}'

When this option is set then `Provision()` and `Grant()` will set above policy for the OBC controller. Similarly remove this policy when it is `Revoked()`. All the existing/old object won't be encrypted unless user overwrites it and while fetching this data RGW won't use decryption. An extended attribute will be set on encrypted objects in a bucket.
```
## Config options
Before this design is implemented, the user could manually set below options from the toolbox pod to configure with `SSE-S3` options for RGW.
```
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_backend vault
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_secret_engine <kv|transit> # https://docs.ceph.com/en/latest/radosgw/vault/#vault-secrets-engines
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_addr <vault address>
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_auth token
ceph config set <ceph auth user for rgw> rgw_crypt_sse_s3_vault_token_file <location of file containing token for accessing vault>
```

The `ceph auth user for rgw` is the (ceph client user)[https://docs.ceph.com/en/latest/rados/operations/user-management/#user-management] who has permissions change the settings for RGW server. This can be listed from `ceph auth ls` command and in Rook ceph client user for RGW will always begins with `client.rgw`, followed by `store` name.
