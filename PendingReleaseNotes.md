# Major Themes

## Action Required

## Notable Features

## Breaking Changes

### Removal of the API service and rookctl tool
The [REST API service](https://github.com/rook/rook/issues/1122) has been removed. All cluster configuration is now accomplished through the 
[CRDs](https://rook.io/docs/rook/master/crds.html) or with the Ceph tools in the [toolbox](https://rook.io/docs/rook/master/toolbox.html). 

The tool `rookctl` has been removed from the toolbox pod. Cluster status and configuration can be queried and changed with the Ceph tools. 
Here are some sample commands to help with your transition.

 `rookctl` Command | Replaced by | Description
 --- | --- | --- 
`rookctl status` | `ceph status` | Query the status of the storage components
`rookctl block` | See the [Block storage](Documentation/block.md) and [direct Block](Documentation/direct-tools.md#block-storage-tools) config | Create, configure, mount, or unmount a block image
`rookctl filesystem` | See the [Filesystem](Documentation/filesystem.md) and [direct File](Documentation/direct-tools.md#shared-filesystem-tools) config | Create, configure, mount, or unmount a file system
`rookctl object` | See the [Object storage](Documentation/object.md) config | Create and configure object stores and object users

## Known Issues

## Deprecations
