# Openshift API Data Protection - Dashboard

- To install this dashboard on Grafana

```shell
oc create -f oadp-dash.yaml
```

&nbsp;

- Mai metrics used in this dashboard

```bash
velero_backup_total
velero_backup_success_total
velero_backup_attempt_total
velero_backup_deletion_success_total
velero_backup_deletion_attempt_total
velero_backup_deletion_failure_total
velero_backup_failure_total
velero_backup_partial_failure_total
velero_backup_tarball_size_bytes
velero_backup_duration_seconds_bucket
velero_restore_total
velero_restore_success_total
velero_restore_attempt_total
velero_restore_failed_total
velero_restore_partial_failure_total
velero_restore_validation_failed_total
velero_volume_snapshot_success_total
velero_volume_snapshot_attempt_total
velero_volume_snapshot_failure_total
```

&nbsp;

![](dash.png)
