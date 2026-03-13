# Tenant Identity Binding for RGW User Accounts

Automatically creates Ceph RGW User Accounts for OpenShift Projects with identity binding.

## Quick Start

Add annotation to an OpenShift Project:

```yaml
apiVersion: project.openshift.io/v1
kind: Project
metadata:
  name: myproject
  annotations:
    object.fusion.io/identity-binding: "true"
```

The controller creates:
- RGW User Account (Ceph 8.1+)
- OIDC provider linked to cluster
- IAM role for STS AssumeRoleWithWebIdentity
- Service account `rgw-identity` in the namespace

Result:
```yaml
annotations:
  object.fusion.io/account-arn: "RGWmyproject"
  object.fusion.io/role-arn: "arn:aws:iam::RGWmyproject:role/project-role"
```

## Files

- `controller.go` - Watches Projects, reconciles identity bindings
- `account.go` - RGW User Account operations via radosgw-admin
- `policy.go` - IAM policy management and policy generators
- `oidc.go` - OIDC configuration and integration with OpenShift

## Key Functions

### account.go
- `CreateAccount(c, accountName)` - Creates RGW User Account
- `GetAccount(c, accountName)` - Gets account info
- `DeleteAccount(c, accountName)` - Deletes account
- `CreateOIDCProvider(c, accountName, issuer, thumbprints)` - Configures OIDC
- `CreateRole(c, accountName, roleName, policyDoc)` - Creates IAM role

### policy.go
- `CreatePolicy(c, accountName, policyName, policyDoc)` - Creates IAM policy
- `AttachRolePolicy(c, accountName, roleName, policyARN)` - Attaches policy to role
- `DetachRolePolicy(c, accountName, roleName, policyARN)` - Detaches policy from role
- `DeletePolicy(c, accountName, policyARN)` - Deletes IAM policy
- `GenerateFullAccessPolicy(accountName)` - Generates full S3 access policy
- `GenerateReadOnlyPolicy(accountName, bucketPrefix)` - Generates read-only policy
- `GenerateBucketSpecificPolicy(accountName, bucket, actions)` - Bucket-specific policy
- `GeneratePathBasedPolicy(accountName, bucket, path, actions)` - Path-based policy

### oidc.go
- `GetClusterOIDCConfig(ctx, k8sClient)` - Retrieves cluster OIDC configuration
- `getServiceAccountIssuer(ctx, k8sClient)` - Gets SA issuer URL
- `getOIDCThumbprints(issuerURL)` - Gets certificate thumbprints
- `VerifyOIDCConfiguration(issuerURL)` - Verifies OIDC setup
- `CreateServiceAccountWithOIDCAnnotations(ctx, k8sClient, ns, account, roleARN)` - Creates SA

### controller.go
- `Reconcile()` - Main reconciliation loop
- `createRGWAccount()` - Creates account and updates annotations
- `verifyRGWAccount()` - Verifies account exists
- `cleanupRGWAccount()` - Deletes account on project deletion

## RGW Commands Used

```bash
# Create account
radosgw-admin account create --account-name=RGWmyproject

# Get info
radosgw-admin account info --account-name=RGWmyproject

# Delete
radosgw-admin account rm --account-name=RGWmyproject

# OIDC provider
radosgw-admin oidc-provider create --account-name=RGWmyproject --issuer=<url> --thumbprint=<hash>

# IAM role
radosgw-admin role create --account-name=RGWmyproject --role-name=project-role --assume-role-policy-doc='<json>'
```

## TODO

- [ ] Wire up to actual Ceph cluster context
- [ ] Get OIDC thumbprint from cluster
- [ ] Add finalizers for cleanup
- [ ] Unit tests
- [ ] Integration tests

## Next Steps

After this PR, we'll implement:
1. Ceph object roles/policies for fine-grained access
2. Full OIDC integration with OpenShift token issuer