module github.com/open-cluster-management/metrics-collector

go 1.16

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/go-kit/kit v0.10.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.2
	github.com/golang/snappy v0.0.1
	github.com/oklog/run v1.1.0
	github.com/open-cluster-management/multicluster-monitoring-operator v0.0.0-20210216210616-0f181640bb3a
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.14.0
	github.com/prometheus/prometheus v2.3.2+incompatible
	github.com/spf13/cobra v1.0.0
	k8s.io/apimachinery v0.19.2
	k8s.io/client-go v12.0.0+incompatible
	sigs.k8s.io/controller-runtime v0.6.0
)

replace (
	github.com/jetstack/cert-manager => github.com/open-cluster-management/cert-manager v0.0.0-20200821135248-2fd523b053f5
	github.com/prometheus/common => github.com/prometheus/common v0.9.1
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190424153033-d3245f150225
	golang.org/x/text => golang.org/x/text v0.3.5
	k8s.io/client-go => k8s.io/client-go v0.19.0
)
