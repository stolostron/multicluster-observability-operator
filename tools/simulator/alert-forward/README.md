# Alert Forward Simulator

The alert forward simulator can be used to simulate multiple Prometheus instances to forward alerts to the Alertmanager in the ACM hub cluster.

## Prereqs

You must meet the following requirements to setup metrics collector:

1. ACM 2.3+ available
2. `MultiClusterObservability` instance available in the hub cluster

## How to use

1. Export host of the Alertmanager in the ACM hub cluster.

```
export ALERTMANAGER_HOST=$(oc -n open-cluster-management-observability get route alertmanager -o jsonpath="{.spec.host}")
```

2. Export access token to the Alertmanager in the ACM hub cluster.

```
export ALERRTMANAGER_ACCESS_TOKEN=$(oc -n open-cluster-management-observability get secret $(oc -n open-cluster-management-observability get sa observability-alertmanager-accessor -o yaml | grep observability-alertmanager-accessor-token | cut -d' ' -f3) -o jsonpath="{.data.token}" | base64 -d)
```

3. (Optional)Export simulated max go routine number for sending alert, if not set, default value(20) will be used.

```
export MAX_ALERT_SEND_ROUTINE=5
```

4. (Optional) Export alert send interval, if not set, default value(5 seconds) will be used.

```
export ALERT_SEND_INTERVAL=10s
```

5. Run the simulator to send fake alerts to the Alertmanager in the ACM hub cluster.

```
# go run ./tools/simulator/alert-forward/main.go
2021/10/12 04:22:50 sending alerts with go routine 0
2021/10/12 04:22:50 conn was reused: false
2021/10/12 04:22:50 send routine 0 done
2021/10/12 04:22:55 sending alerts with go routine 1
2021/10/12 04:22:55 conn was reused: true
2021/10/12 04:22:55 send routine 1 done
2021/10/12 04:23:00 sending alerts with go routine 2
2021/10/12 04:23:00 conn was reused: true
2021/10/12 04:23:00 send routine 2 done
2021/10/12 04:23:05 sending alerts with go routine 3
2021/10/12 04:23:05 conn was reused: true
2021/10/12 04:23:05 send routine 3 done
2021/10/12 04:23:10 sending alerts with go routine 4
2021/10/12 04:23:10 conn was reused: true
2021/10/12 04:23:10 send routine 4 done
2021/10/12 04:23:15 sending alerts with go routine 5
2021/10/12 04:23:15 conn was reused: true
2021/10/12 04:23:15 send routine 5 done
```

