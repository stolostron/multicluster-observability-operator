module github.com/stolostron/multicluster-observability-operator

go 1.18

require (
	github.com/IBM/controller-filtered-cache v0.3.3
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cloudflare/cfssl v1.6.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-kit/kit v0.11.0
	github.com/go-kit/log v0.1.0
	github.com/go-logr/logr v0.4.0
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.2
	github.com/golang/snappy v0.0.4
	github.com/hashicorp/go-version v1.3.0
	github.com/oklog/run v1.1.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/client-go v0.0.0-20210916133943-9acee1a0fb83
	github.com/openshift/cluster-monitoring-operator v0.1.1-0.20210611103744-7168290cd660
	github.com/pkg/errors v0.9.1
	github.com/prometheus-community/prom-label-proxy v0.3.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.48.1
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.47.1
	github.com/prometheus/alertmanager v0.22.2
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.30.0
	github.com/prometheus/prometheus v2.3.2+incompatible
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/stolostron/multiclusterhub-operator v0.0.0-20220106205009-2af6f43fd562
	github.com/stolostron/observatorium-operator v0.0.0-20220307015247-f9eb849e218e
	github.com/stretchr/testify v1.7.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.1
	k8s.io/apiextensions-apiserver v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v13.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.21.2
	open-cluster-management.io/addon-framework v0.0.0-20211014025435-1f42884cdd53
	open-cluster-management.io/api v0.0.0-20210916013819-2e58cdb938f9
	sigs.k8s.io/controller-runtime v0.10.0
	sigs.k8s.io/kube-storage-version-migrator v0.0.4
	sigs.k8s.io/kustomize/api v0.8.8
	sigs.k8s.io/kustomize/v3 v3.3.1
	sigs.k8s.io/yaml v1.2.0
)

require (
	github.com/armon/go-metrics v0.3.10 // indirect
	github.com/containerd/containerd v1.5.10 // indirect
	github.com/docker/distribution v2.8.0+incompatible // indirect
	github.com/emicklei/go-restful v2.14.2+incompatible // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/hashicorp/consul/api v1.10.0 // indirect
	github.com/hashicorp/go-hclog v0.14.1 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.0 // indirect
	github.com/stretchr/objx v0.3.0 // indirect
	golang.org/x/net v0.0.0-20211209124913-491a49abca63 // indirect
	k8s.io/kube-openapi v0.0.0-20210929172449-94abcedd1aa4 // indirect
)

replace (
	github.com/go-openapi/analysis => github.com/go-openapi/analysis v0.19.5
	github.com/go-openapi/loads => github.com/go-openapi/loads v0.19.5
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.19.5
	github.com/hashicorp/consul => github.com/hashicorp/consul v1.10.10
	github.com/nats-io/nats-server/v2 => github.com/nats-io/nats-server/v2 v2.7.2
	github.com/openshift/api => github.com/openshift/api v0.0.0-20210331193751-3acddb19d360
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v1.8.2-0.20210811141203-dcb07e8eac34
	golang.org/x/text => golang.org/x/text v0.3.5
	k8s.io/client-go => k8s.io/client-go v0.22.1
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
)

// needed because otherwise installer fetches a library-go version that requires bitbucket.com/ww/goautoneg which is dead
// Tagged version fetches github.com/munnerz/goautoneg instead
replace github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20210916194400-ae21aab32431
