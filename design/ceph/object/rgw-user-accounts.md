# CephObjectStore User Accounts

## Overview
The Ceph Object Gateway added support for user accounts in Squid (v19.0.0) release. This as an optional feature to enable the self-service management of Users, Groups and Roles, similar to those in AWS Identity and Access Management (IAM), which helps enhancing Ceph Multitenancy.

Now instead of each S3 user operating independently, an account groups users and roles under shared ownership. Resources like buckets and objects are owned by the account, not individual users. This brings major advantages:

- *Aggregated quotas & stats*: Track usage and enforce limits at the account level.
- *Shared visibility*: Every user or role in the account can list and manage account‑owned buckets.
- *Streamlined management*: Apply IAM policies centrally throughout the account for consistent permissions.

## Prerequisites
- Ceph Squid (v19.0.0) or later
- RGW/Object Storage Service should be running.

## API Changes
- A new CephObjectStoreAccount resource will represent a user account

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreAccount
metadata:
  # resource name will be used as unique account name while creating the IAM account
  name: my-account
  namespace: rook-ceph
spec:
  # [Required] The name of the object store to create the account in
  store: my-store
  # [Optional]: The desired name of the account if different from the CephObjectStoreAccount CR name.
  name: my-account
  # [Optional] Uniquely identifies an account and resource ownership. Format should be RGW followed by 17 digits (e.g.,
  # RGW00889737169837717). If not specified, then ceph will auto generate the account ID.
  accountID: RGW33567154695143645
  # [Optional] Email address associated with the account
  email: admin@example.com
  # [Optional] Root user for the account. The root user is created by default and has default
  # permissions across all account resources. It can manage IAM users, roles, and policies.
  rootUser:
    # [Optional] If set to true, the root user will not be created for this account. This can be
    # useful if the user wants to manually manage the root user outside of Rook. Default: false.
    skipCreate: false
    # [Optional] Display name for the root user
    displayName: "Root User for Rook Account <namespace>/<name>"
status:
  phase: Ready/Failure
  # accountID of the IAM account. Adding account id to the status will help to get a quick reference to the account in case the user does not provide the account ID in the spec.
  accountID: RGW33567154695143645
  # Reference to the Kubernetes secret containing the root user's access credentials
  rootAccountSecretName: rook-ceph-object-user-my-store-my-account
```

## Account Creation
- A new controller will watch for create, update and delete requests on the CephObjectStoreAccount resource.
- The controller will create the account if it does not exist.
- If `spec.name` is provided, then it will be used to create the account. Otherwise, `metadata.name` will be used.
- If `spec.accountID` is provided, then it will be used to create the account.
- If `spec.email` is provided, it will be included in the account creation.
- After account creation, the controller will create the root user for the account by default, unless `spec.rootUser.skipCreate` is set to `true`.
- The root user's UID will be the `metadata.uid` (Kubernetes-generated UUID) of the `CephObjectStoreAccount` CR. This ensures global uniqueness across multi-cluster and multisite environments, avoiding conflicts when the same namespace/name combination exists on different clusters.

The controller will use the RGW admin ops API to create the account and root user. This ensures a single implementation that works for both internal and external RGW deployments.

1. Create the account via the admin ops API with the account name, optional account ID, and optional email.
2. If `spec.rootUser.skipCreate` is not `true`, create the root user via the admin ops API with the UID (`metadata.uid`), display name, account ID, and the account root flag. Access key and secret key will be auto-generated.

If the root user is created, its access credentials (access key and secret key) will be stored in a Kubernetes secret, similar to how `CephObjectStoreUser` credentials are managed.

## Account Update
The controller will reconcile any changes made to the CephObjectStoreAccount resource and apply them to the underlying RGW account.

### Immutable Fields
- Account ID (`spec.accountID`) is immutable and cannot be updated after account creation.

### Updatable Fields
Users can update the following attributes of the account in the spec:
- **name** - The account name.
- **email** - Email address associated with the account
- **rootUser.displayName** - Display name for the root user

### Update Operations
When a CephObjectStoreAccount resource is updated, the controller will:

1. **Account Metadata Updates**: Update account name or email if modified (subject to RGW capabilities)

2. **Root User Updates**: Update root user display name if modified via the admin ops API.

3. **Validation**: The controller will validate updates and report errors in the status if:
   - Attempting to modify immutable fields (accountID)
   - RGW returns errors during update operations

## Account Deletion
- A delete request on the `CephObjectStoreAccount` resource will trigger the operator reconcile to delete the account and its root user.
- If the root user was created (i.e., `spec.rootUser.skipCreate` is not `true`), the controller will first delete the root user via the admin ops API.
- Then delete the account itself via the admin ops API.
- Customer should ensure that all the additional users and buckets associated with the account are deleted before the deletion of the account or else the account deletion will fail.

## Root Account Users

### Overview
An account in Ceph Object Gateway is managed by a designated "account root user." This administrator-created entity serves as the primary manager for all resources within that account, including users, groups, and roles.

Root account users have default permissions across all account resources. Their access credentials (keys) enable management through the IAM API to create additional IAM users and roles for use with the Ceph Object Gateway S3 API, as well as to manage their associated access keys and policies.

The root user has a 1:1 relationship with the account and is created automatically by default as part of the `CephObjectStoreAccount` CR lifecycle. Users can opt out of automatic root user creation by setting `spec.rootUser.skipCreate` to `true`.

### Root User Specification
The root user is defined within the `CephObjectStoreAccount` spec:

```yaml
spec:
  rootUser:
    # [Optional] If set to true, the root user will not be created for this account. Default: false.
    skipCreate: false
    # [Optional] Display name for the root user
    displayName: "Root User for Rook Account <namespace>/<name>"
```

### Root User Privileges
Root account users have the following privileges:
- Default permissions across all account resources
- Ability to create and manage IAM users and roles via the IAM API
- Ability to manage access keys and policies for the account
- Can function without explicit IAM policies (though Deny statements can still block root access)

### Example: Creating an Account with Root User

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreAccount
metadata:
  name: my-account
  namespace: rook-ceph
spec:
  store: my-store
  accountID: RGW33567154695143645
  email: admin@example.com
  rootUser:
    displayName: "Root User for Rook Account rook-ceph/my-account"
```

This single CR will:
1. Create the RGW account with the specified account ID and email
2. Create a root user for the account with the given display name
3. Generate access credentials and store them in a Kubernetes secret
4. Report the account ID and secret name in `status.rootAccountSecretName`

## Account Users

### Overview
In addition to the root user (managed by the `CephObjectStoreAccount` CR), accounts can have regular users associated with them. These account users differ from standalone RGW users in following ways:

- Account users start with **no default permissions** (unlike standalone users who can create buckets and upload objects by default).
- Resources created by account users are **owned by the account**, not the individual user.
- Account users are referenced by their display name in IAM policy ARNs, and the display name must match the pattern `[\w+=,.@-]+` for IAM compatibility.

The existing `CephObjectStoreUser` CR is extended with an optional `accountRef` field to associate a user with an account.

### API Changes

The `CephObjectStoreUser` spec gains a new `accountRef` field:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  # [Required] The name of the object store to create the user in
  store: my-store
  # [Required] Display name for the user. Must match [\w+=,.@-]+ when accountRef is set.
  displayName: "my-user"
  # [Optional] Reference to the CephObjectStoreAccount to associate this user with.
  # The account must be in the same namespace as the user.
  accountRef:
    # [Required] Name of the CephObjectStoreAccount CR
    name: my-account
```

The `accountRef` is a reference to a `CephObjectStoreAccount` CR, not a raw account ID. The referenced account must be in the same namespace as the user. Users deploy the account and user CRs simultaneously without needing to wait for the account to be provisioned first.

### Account User Creation

When a `CephObjectStoreUser` with `accountRef` is created, the user controller will:

1. **Resolve the account reference**: Look up the `CephObjectStoreAccount` CR by the name specified in `accountRef`, in the same namespace as the `CephObjectStoreUser` CR.

2. **Validate the object store**: Ensure the `store` field on the user matches the `store` field on the referenced account. If they differ, the controller sets the user status to `Failed` with an appropriate error message.

4. **Wait for the account to be ready**: If the referenced `CephObjectStoreAccount` does not exist or is not in `Ready` phase, the controller will requeue the reconciliation rather than failing. This supports workflows where the account and user CRs are deployed simultaneously.

5. **Validate the display name**: When `accountRef` is set, the controller validates that `displayName` matches the pattern `[\w+=,.@-]+`. This is required because account users are referenced by their display name in IAM policy ARNs (e.g., `arn:aws:iam::RGW33567154695143645:user/my-user`), and names with spaces or unsupported characters will break IAM policy resolution. If the display name is invalid, the controller sets the user status to `Failed` with an appropriate error message. This validation is performed in the controller code rather than at the CRD level, because adding a CRD-level pattern constraint on `displayName` would break existing standalone `CephObjectStoreUser` CRs that have display names with spaces or special characters.

6. **Create the user with the account ID**: Once the account is ready, the controller extracts the `accountID` from the account's `status.accountID` and passes it to the RGW admin ops API when creating or modifying the user.

### Immutable Fields
- `accountRef` is **immutable** once set. Moving a user between accounts changes resource ownership in Ceph and is a destructive operation that must be done manually.
- Immutability is enforced at the CRD level using a CEL (Common Expression Language) validation rule on the field, consistent with how Rook enforces immutability for other fields like `accountID` and `store`:
  ```go
  // +kubebuilder:validation:XValidation:message="accountRef is immutable",rule="self == oldSelf"
  ```
- With this approach, the Kubernetes API server rejects any update that attempts to change `accountRef` before it reaches the controller. This includes changing the name, adding `accountRef` to an existing user, or removing it.

### Account User Deletion

When a `CephObjectStoreUser` with `accountRef` is deleted:
- The controller deletes the user from the RGW via the admin ops API, same as for standalone users.
- The account itself is not affected. Deleting the user simply removes it from the account.

### Example: Full Account Setup with Users

```yaml
# 1. Create the account
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreAccount
metadata:
  name: my-account
  namespace: rook-ceph
spec:
  store: my-store
  rootUser:
    displayName: "Root User"
---
# 2. Create a user associated with the account
apiVersion: ceph.rook.io/v1
kind: CephObjectStoreUser
metadata:
  name: my-user
  namespace: rook-ceph
spec:
  store: my-store
  displayName: "my-user"
  accountRef:
    name: my-account
```

Both CRs can be applied simultaneously. The user controller will wait for the account to become ready before creating the user in RGW.

### External Cluster Considerations

For external RGW clusters, the `accountRef` approach works as long as the account is managed via a `CephObjectStoreAccount` CR — the account controller uses the RGW admin ops API (HTTP), which works for both internal and external RGW deployments.

Associating users with pre-existing accounts that were created outside of Rook (e.g., directly via `radosgw-admin`) is not supported in this iteration. Future work may add external binding support to `CephObjectStoreAccount`, allowing a CR to adopt an existing account by its account ID. Once bound, the `accountRef` mechanism works the same way — the user controller always resolves the account ID from the referenced CR's status.

### Future Considerations

- **Migrating standalone users into accounts**: In the future, all users may need to be associated with an account. This would require allowing `accountRef` to be added to existing standalone `CephObjectStoreUser` CRs as a day-2 operation. The CEL immutability rule would need to be relaxed from `self == oldSelf` to `!has(oldSelf) || self == oldSelf` to permit the transition from unset to set while still preventing changes or removal once set. Additionally, the current `CephObjectStoreUser` CRD has no validation on `displayName` — standalone users can have display names with spaces or special characters. However, account users are referenced by display name in IAM policy ARNs and must match `[\w+=,.@-]+`. Since adding a CRD-level pattern constraint would break existing standalone users, the display name validation must be enforced in the controller only when `accountRef` is set. Users migrating into an account would need to update their `displayName` to be IAM-compatible before or at the same time as setting `accountRef`.
- **Cross-namespace account references**: Currently, `CephObjectStoreUser` must be in the same namespace as the referenced `CephObjectStoreAccount`. In the future, cross-namespace references could be supported by adding a `namespace` field to `accountRef` along with an opt-in mechanism on the `CephObjectStoreAccount` (e.g., `allowUsersFromNamespaces` list) to let the account owner control which namespaces can create users for their account. This prevents unauthorized cross-namespace association where any user could claim to belong to an account they don't own.
- **External account binding**: Support for `CephObjectStoreAccount` to adopt pre-existing accounts in external RGW clusters by referencing a raw account ID.
