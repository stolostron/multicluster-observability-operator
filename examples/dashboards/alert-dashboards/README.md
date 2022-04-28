# Alert Dashboards

Included in this pack are 3 experimental dashboards meant to give an overview of Alerts:
- [Alert Analysis](alert-analysis.png) - the overview dashboard containing both current and historical status with drill downs into the dashboards shown below.
- [Clusters by Alert](cluster-by-alerts.png) - choose alerts and see clusters effected in time.
- [Alerts by Cluster](alerts-by-cluster.png) - choose a cluster and see alerts firing on that cluster in time.

Known Limitations: 
1. These dashboards work well if used with ACM 2.4 where the Grafana Version is 8.*.
1. These dashboards are not aware of alerts that may have been suppressed in the Alertmanager configuration of ACM
1. There is one alert: ViolatedPolicyReport that appears without a cluster name in the dashboards. This will be addressed soon.
