# Alert Forward Simulator

The alert forward simulator can be used to simulate multiple Prometheus instances to forward alerts concurrently to the Alertmanager in the ACM hub cluster.

## Prereqs

You must meet the following requirements to setup metrics collector:

1. ACM 2.3+ available
2. `MultiClusterObservability` instance available in the hub cluster

## How to use

### Run locally outside the cluster

1. Export host of the Alertmanager of ACM hub cluster:

```
export ALERTMANAGER_HOST=$(oc -n open-cluster-management-observability get route alertmanager -o jsonpath="{.spec.host}")
```

2. Export access token to the Alertmanager of ACM hub cluster:

```
export ALERRTMANAGER_ACCESS_TOKEN=$(oc -n open-cluster-management-observability get secret $(oc -n open-cluster-management-observability get sa observability-alertmanager-accessor -o yaml | grep observability-alertmanager-accessor-token | cut -d' ' -f3) -o jsonpath="{.data.token}" | base64 -d)
```

3. Run the simulator to send simulated alerts to the Alertmanager of ACM hub cluster:

```
$ go run main.go --am-host=${ALERTMANAGER_HOST} --am-access-token=${ALERRTMANAGER_ACCESS_TOKEN} --alerts-file=./alerts.json
2021/11/08 07:03:23 alert forwarder is initialized
2021/11/08 07:03:23 starting alert forward loop....
2021/11/08 07:03:53 sending alerts with worker 0
2021/11/08 07:03:53 sending alerts with worker 1
...
```

> Note: you can also optionally specify the simulated alerts that forwarded by `--alerts-file` flag.

4. Specify the number of concurrent workers that forward the alerts by `--workers` flag, the default value is `1000`:

```
$ go run main.go --am-host=${ALERTMANAGER_HOST} --am-access-token=${ALERRTMANAGER_ACCESS_TOKEN} --alerts-file=./alerts.json --workers 3
2021/11/08 07:03:23 alert forwarder is initialized
2021/11/08 07:03:23 starting alert forward loop....
2021/11/08 07:03:53 sending alerts with worker 0
2021/11/08 07:03:53 sending alerts with worker 1
2021/11/08 07:03:53 sending alerts with worker 2
2021/11/08 07:03:54 connection was reused: false
2021/11/08 07:03:54 connection was reused: false
2021/11/08 07:03:54 send routine 0 done
2021/11/08 07:03:54 send routine 2 done
2021/11/08 07:03:54 send routine 1 done
```

5. Optionally specify the alert forward interval by `--interval` flag, default value is `30s`:

```
$ go run main.go --am-host=${ALERTMANAGER_HOST} --am-access-token=${ALERRTMANAGER_ACCESS_TOKEN} --alerts-file=./alerts.json --workers 3 --interval 5s
2021/11/08 07:08:12 alert forwarder is initialized
2021/11/08 07:08:12 starting alert forward loop....
2021/11/08 07:08:17 sending alerts with worker 0
2021/11/08 07:08:17 sending alerts with worker 1
2021/11/08 07:08:17 sending alerts with worker 2
2021/11/08 07:08:17 connection was reused: false
2021/11/08 07:08:17 connection was reused: false
2021/11/08 07:08:17 connection was reused: false
2021/11/08 07:08:17 send routine 0 done
2021/11/08 07:08:17 send routine 1 done
2021/11/08 07:08:17 send routine 2 done
2021/11/08 07:08:22 sending alerts with worker 0
2021/11/08 07:08:22 sending alerts with worker 1
2021/11/08 07:08:22 sending alerts with worker 2
2021/11/08 07:08:22 connection was reused: true
2021/11/08 07:08:22 connection was reused: true
2021/11/08 07:08:22 connection was reused: true
2021/11/08 07:08:22 send routine 0 done
2021/11/08 07:08:22 send routine 1 done
2021/11/08 07:08:22 send routine 2 done
^C2021/11/08 07:08:29 got unix terminating signal: interrupt
2021/11/08 07:08:29 received terminating signal, shuting down the program...
```

### Run as a Deployment inside the cluster

1. (Optional) Build and push the alert-forwarder image:
```
docker build -f Dockerfile -t quay.io/ocm-observability/alert-forwarder:2.4.0 ../../..
docker push quay.io/ocm-observability/alert-forwarder:2.4.0
```

2. Run the following command to deploy the alert-forwarder:

```
./setup-alert-forwarder.sh
```

3. Check if all the alert-forwarder pod is running successfully in your cluster:

```
$ oc -n alert-forwarder get pod
NAME                              READY   STATUS    RESTARTS   AGE
alert-forwarder-fb75bbb8c-6zgq8   1/1     Running   0          3m11s
$ oc -n alert-forwarder logs -f alert-forwarder-fb75bbb8c-6zgq8
2021/11/08 07:25:54 alert forwarder is initialized
2021/11/08 07:25:54 starting alert forward loop....
2021/11/08 07:26:24 sending alerts with worker 0
2021/11/08 07:26:24 sending alerts with worker 1
...
```

4. Specify the number of concurrent workers that forward the alerts by `-w` flag, the default value is `1000`:

```
$ ./setup-alert-forwarder.sh -w 3
$ oc -n alert-forwarder logs -f deploy/alert-forwarder
2021/11/08 07:53:07 alert forwarder is initialized
2021/11/08 07:53:07 starting alert forward loop....
2021/11/08 07:53:37 sending alerts with worker 0
2021/11/08 07:53:37 sending alerts with worker 1
2021/11/08 07:53:37 sending alerts with worker 2
2021/11/08 07:53:37 connection was reused: false
2021/11/08 07:53:37 connection was reused: false
2021/11/08 07:53:37 connection was reused: false
2021/11/08 07:53:37 send routine 0 done
2021/11/08 07:53:37 send routine 2 done
2021/11/08 07:53:37 send routine 1 done
...
```

5. Optionally specify the alert forward interval by `-i` flag, default value is `30s`:

```
$ ./setup-alert-forwarder.sh -w 3 -i 5s
$ oc -n alert-forwarder logs -f deploy/alert-forwarder
2021/11/08 07:57:23 alert forwarder is initialized
2021/11/08 07:57:23 starting alert forward loop....
2021/11/08 07:57:28 sending alerts with worker 0
2021/11/08 07:57:28 sending alerts with worker 1
2021/11/08 07:57:28 sending alerts with worker 2
2021/11/08 07:57:28 connection was reused: false
2021/11/08 07:57:28 connection was reused: false
2021/11/08 07:57:28 connection was reused: false
2021/11/08 07:57:28 send routine 2 done
2021/11/08 07:57:28 send routine 0 done
2021/11/08 07:57:28 send routine 1 done
2021/11/08 07:57:33 sending alerts with worker 0
2021/11/08 07:57:33 sending alerts with worker 1
2021/11/08 07:57:33 sending alerts with worker 2
2021/11/08 07:57:33 connection was reused: true
2021/11/08 07:57:33 connection was reused: true
2021/11/08 07:57:33 connection was reused: true
2021/11/08 07:57:33 send routine 2 done
2021/11/08 07:57:33 send routine 1 done
2021/11/08 07:57:33 send routine 0 done
...
```

