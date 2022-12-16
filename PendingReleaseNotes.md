# v1.11 Pending Release Notes

## Breaking Changes

- Removed support for MachineDisruptionBudgets, including settings removed from the CephCluster CR:
  - `manageMachineDisruptionBudgets`
  - `machineDisruptionBudgetNamespace`

## Features

- Change `pspEnable` default value to `false` in helm charts.
