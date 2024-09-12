# Grafana Dashboards

This folder contains the JSON files for the Grafana dashboards.
The dashboards are based upon [the official Ceph Grafana dashboards](https://github.com/ceph/ceph/tree/main/monitoring/ceph-mixin) but with some slight tweaks.

## Updating the Dashboards

To update the dashboards, please export them via Grafana's built-in export function.

Please note that exporting a dashboard from Grafana, that version of Grafana will be the minimum required version for all users. For example, exporting from Grafana 9.1.0 would require all users to also have at least Grafana version 9.1.0 running.

So it might be a good idea to take that into account when updating the dashboards.
