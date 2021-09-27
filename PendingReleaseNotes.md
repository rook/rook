# Major Themes

v1.8...

## K8s Version Support

## Upgrade Guides

## Breaking Changes

- Flex driver is fully deprecated. If you are still using flex volumes, before upgrading to v1.8
  you will need to convert them to csi volumes. See the flex conversion tool.
- Min supported version of K8s is now 1.16. If running on an older version of K8s it is recommended
  to update to a newer version before updating to Rook v1.8.

## Features

- The Rook Operator does not use "tini" as an init process. Instead, it uses the "rook" and handles
  signals on its own.
