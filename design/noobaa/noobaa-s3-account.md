♜ [Rook NooBaa Design](README.md) /
# S3 Account

Native S3 applications need credentials (access-key, secret-key) and an endpoint in order to use the S3 API from the application.

In order to provide applications these credentials the application will set annotations `s3-account.noobaa.rook.io/*` as shown in the examples below, to request that the operator provide a secret with S3 credentials.

The annotations will be added to the service-account in order to create a matching identity in NooBaa and provide a secret with S3 credentials in the application namespace. The operator will fulfill the request by creating a NooBaa account and put the access-key, secret-key, and endpoint in a secret for the application.

Once the service-account is removed or the annotations are removed, the operator will delete the NooBaa account and the secret.

# Example

The annotations on the service-account:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: app-account
  namespace: app-namespace
  # These annotations are requesting the operator to create a secret called `s3-credentials`
  # such that `noobaa-default-class` is used for new buckets.
  annotations:    
    s3-account.noobaa.rook.io/secret-name: s3-credentials
    s3-account.noobaa.rook.io/bucket-class: noobaa-default-class
```

The operator will create a secret like this:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: s3-credentials
  namespace: app-namespace
type: Opaque
data:
  AWS_ACCESS_KEY_ID: XXXXX
  AWS_SECRET_ACCESS_KEY: YYYYY
  S3_ENDPOINT: ZZZZZ
```

And the application deployments can load it to pods like this:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-deployment
spec:
  replicas: 42
  template:
    spec:
      containers:
        - name: app-container
          # Here we map the secret to the container env
          envFrom:
            - secretRef:
                name: s3-credentials
```
