# Ceph Lua Script

## Use case

The Ceph Lua script was created to provide a custom resource for managing Lua scripts on RGW dynamically at runtime using the `radosgw-admin script` commands.

### Why do we need this controller in Rook

With the Ceph Lua Script controller, the `CephLuaScript` maintains the state of a Lua script on RGW with its lifecycle tied to the creation, update, and deletion of the custom resource.

- On Create, applies a script to RGW with the `radosgw-admin script put` command.
- On Delete, deletes a script from RGW with the `radosgw-admin script rm` command.
- On Update, applies a create/delete on the new/old Lua script respectively.
- On `CephObjectStore` changes, the Delete/Create will be applied across old (if possible) and new object stores, respectively.
- On `CephObjectStore` restored, an annotation change on the `CephLuaScript` will reconcile the Lua script state once again.

Usage information for the `radosgw-admin script` can be found in the docs https://docs.ceph.com/en/reef/radosgw/lua-scripting/.

## Implementation details

The Lua script controller will reconcile the `CephLuaScript` custom resource to sync the custom resource state against an RGW loaded script.

- A (weak) pointer to a `CephObjectStore` is added through the `.spec.objectStoreName` and `.spec.objectStoreNamespace` string fields. The controller will need a handle of the store to obtain a zone name used during RGW upload.
- The Lua script content is provided through one of three methods written to the operator filesystem and optionally copied over network to a Multus configured Pod prior to uploading to RGW:
  1. `.spec.script` (string)
  2. `.spec.scriptBase64` (base64 encoded string)
  3. `.spec.scriptURL` (string)
     - The file will need validation on its size to prevent Pod running out of disk space.
  4. The script should be deleted from the local filesystem after it is uploaded to RGW.
- A user can specify a list of secrets (`.spec.env`) and/or secret name (`.spec.envFrom`) to substitute the environment variables into the `.spec.script` (or others) at runtime.
  - Alternative to substituting, Ceph could support a custom flag `radosgw-admin script put --env MY_VAR=<MY_VAR>` during the script upload process. In that case, the Lua script controller would only be responsible for linking the CR fields to the admin command.
  - The Lua script controller will break out of reconcile with status Failed if the Secrets are not found and requeue at a later time.
- A metadata signature will be prepended to each script `-- <name>:<namespace>:<tenant>:<script-hash>` to help with change management.
  - `<name>` - the `CephLuaScript` name
  - `<namespace>` - the `CephLuaScript` namespace
  - `<tenant>` - the `CephLuaScript` tenant from `.spec.tenant`
  - `<script-hash>` - a secure hash of the `CephLuaScript`'s `.spec.script` (or others) and Secret Data content from `.spec.env` and/or `.spec.envFrom`.
- The controller should validate that tenants provided through `.spec.tenant` do not override the global RGW tenant if undefined.
- A script can be applied to each `.spec.context` with the valid contexts outlined in https://docs.ceph.com/en/reef/radosgw/lua-scripting/.
- Changes to `CephLuaScript` and connected env `Secret`'s should start a new reconcile. Once status is marked as Ready, reconciles should not happen again until another user-facing change is made to `CephLuaScript` or env `Secret`'s. In the case where the recovery of the pointed `CephObjectStore` needs to force a new Lua script reconcile, it can be tracked through an annotation on the `CephLuaScript` with the help of both controllers. Failures to connect to RGW should lead to requeuing the CR until a proper connection is established.
- Command line tools can check `.status.observedGeneration` in combination with the Ready status to validate the reconciled state after one or more revisions have been made to the `CephLuaScript`.

### Example

In the following example, two Lua scripts will be created in the Ceph object store `my-store` zone in the `preRequest` and `postRequest` contexts using the global/default tenant.

```yaml
apiVersion: ceph.rook.io/v1
kind: CephLuaScript
metadata:
  name: pre-request-script
  namespace: rook-ceph
spec:
  objectStoreName: my-store
  objectStoreNamespace: rook-ceph
  context: preRequest
  script: |
    RGWDebugLog("preRequest logged...")
---
apiVersion: ceph.rook.io/v1
kind: CephLuaScript
metadata:
  name: post-request-script
  namespace: rook-ceph
spec:
  objectStoreName: my-store
  objectStoreNamespace: rook-ceph
  context: postRequest
  script: |
    RGWDebugLog("postRequest logged...")
```
