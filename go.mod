module github.com/stolostron/multicluster-observability-operator

go 1.20

require (
	github.com/IBM/controller-filtered-cache v0.3.6
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/cloudflare/cfssl v1.6.0
	github.com/ghodss/yaml v1.0.1-0.20190212211648-25d852aebe32
	github.com/go-co-op/gocron v1.23.0
	github.com/go-kit/log v0.2.1
	github.com/go-logr/logr v1.2.4
	github.com/gogo/protobuf v1.3.2
	github.com/golang/protobuf v1.5.3
	github.com/golang/snappy v0.0.4
	github.com/google/go-cmp v0.5.9
	github.com/hashicorp/go-version v1.3.0
	github.com/oklog/run v1.1.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.27.8
	github.com/openshift/api v3.9.1-0.20191111211345-a27ff30ebf09+incompatible
	github.com/openshift/client-go v0.0.0-20230120202327-72f107311084
	github.com/openshift/cluster-monitoring-operator v0.0.0-20230118025836-20fcb9f6ef4e
	github.com/openshift/hypershift v0.1.11
	github.com/prometheus-community/prom-label-proxy v0.6.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.58.0
	github.com/prometheus-operator/prometheus-operator/pkg/client v0.53.1
	github.com/prometheus/alertmanager v0.25.1
	github.com/prometheus/client_golang v1.17.0
	github.com/prometheus/client_model v0.5.0
	github.com/prometheus/common v0.44.0
	github.com/prometheus/prometheus v1.8.2-0.20211214150951-52c693a63be1
	github.com/spf13/cobra v1.7.0
	github.com/spf13/pflag v1.0.6-0.20210604193023-d5e0c0615ace
	github.com/stolostron/multiclusterhub-operator v0.0.0-20220902185016-e81ccfbecf55
	github.com/stolostron/observatorium-operator v0.0.0-20240403132649-1f7129fc3a27
	github.com/stretchr/testify v1.8.4
	github.com/thanos-io/thanos v0.30.0
	go.uber.org/zap v1.26.0
	golang.org/x/exp v0.0.0-20221212164502-fae10dda9338
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.28.2
	k8s.io/apiextensions-apiserver v0.27.2
	k8s.io/apimachinery v0.28.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kubectl v0.27.2
	open-cluster-management.io/addon-framework v0.8.1-0.20231128122622-3bfdbffb237c
	open-cluster-management.io/api v0.12.1-0.20231130134655-97a8a92a7f30
	sigs.k8s.io/controller-runtime v0.15.1
	sigs.k8s.io/kube-storage-version-migrator v0.0.4
	sigs.k8s.io/kustomize/api v0.13.4
	sigs.k8s.io/kustomize/kyaml v0.14.2
	sigs.k8s.io/yaml v1.3.0
)

require (
	github.com/alecthomas/units v0.0.0-20211218093645-b94a6e3cc137 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go v1.44.213 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/brancz/locutus v0.0.0-20210511124350-7a84f4d1bcb3 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/edsrzf/mmap-go v1.1.0 // indirect
	github.com/efficientgo/core v1.0.0-rc.2 // indirect
	github.com/efficientgo/tools/core v0.0.0-20220817170617-6c25e3b627dd // indirect
	github.com/emicklei/go-restful/v3 v3.10.2 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/go-kit/kit v0.12.0 // indirect
	github.com/go-logfmt/logfmt v0.5.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.2.4 // indirect
	github.com/go-openapi/analysis v0.21.4 // indirect
	github.com/go-openapi/errors v0.20.3 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/loads v0.21.2 // indirect
	github.com/go-openapi/runtime v0.25.0 // indirect
	github.com/go-openapi/spec v0.20.7 // indirect
	github.com/go-openapi/strfmt v0.21.7 // indirect
	github.com/go-openapi/swag v0.22.3 // indirect
	github.com/go-openapi/validate v0.22.0 // indirect
	github.com/gobuffalo/flect v1.0.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/certificate-transparency-go v1.0.21 // indirect
	github.com/google/gnostic v0.6.9 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/grafana/regexp v0.0.0-20221122212121-6b5c0a4cb7fd // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.0.0-rc.2.0.20201207153454-9f6bf00c00a7 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jmoiron/sqlx v1.2.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/metalmatze/signal v0.0.0-20210307161603-1c9aa721a97a // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/monochromegane/go-gitignore v0.0.0-20200626010858-205db1a8cc00 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/openshift/library-go v0.0.0-20230120214501-9bc305884fcb // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/uber/jaeger-client-go v2.30.0+incompatible // indirect
	github.com/uber/jaeger-lib v2.4.1+incompatible // indirect
	github.com/weppos/publicsuffix-go v0.13.0 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	github.com/zmap/zcrypto v0.0.0-20201128221613-3719af1573cf // indirect
	github.com/zmap/zlint/v3 v3.0.0 // indirect
	go.mongodb.org/mongo-driver v1.11.4 // indirect
	go.opentelemetry.io/contrib/propagators/autoprop v0.34.0 // indirect
	go.opentelemetry.io/contrib/propagators/aws v1.9.0 // indirect
	go.opentelemetry.io/contrib/propagators/b3 v1.9.0 // indirect
	go.opentelemetry.io/contrib/propagators/jaeger v1.9.0 // indirect
	go.opentelemetry.io/contrib/propagators/ot v1.9.0 // indirect
	go.opentelemetry.io/otel v1.11.2 // indirect
	go.opentelemetry.io/otel/bridge/opentracing v1.10.0 // indirect
	go.opentelemetry.io/otel/sdk v1.11.2 // indirect
	go.opentelemetry.io/otel/trace v1.11.2 // indirect
	go.starlark.net v0.0.0-20200306205701-8dd3e2ee1dd5 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/goleak v1.2.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/mod v0.10.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/oauth2 v0.10.0 // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/term v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.9.3 // indirect
	gomodules.xyz/jsonpatch/v2 v2.3.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1 // indirect
	google.golang.org/grpc v1.56.3 // indirect
	google.golang.org/protobuf v1.32.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/apiserver v0.27.2 // indirect
	k8s.io/component-base v0.27.2 // indirect
	k8s.io/klog/v2 v2.100.1 // indirect
	k8s.io/kube-aggregator v0.26.1 // indirect
	k8s.io/kube-openapi v0.0.0-20230717233707-2695361300d9 // indirect
	k8s.io/utils v0.0.0-20230505201702-9f6742963106 // indirect
	sigs.k8s.io/cluster-api v1.5.1 // indirect
	sigs.k8s.io/cluster-api-provider-aws/v2 v2.0.2 // indirect
	sigs.k8s.io/cluster-api-provider-ibmcloud v0.6.0 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
)

replace (
	github.com/google/gnostic => github.com/google/gnostic v0.5.7-v3refs
	github.com/openshift/api => github.com/openshift/api v0.0.0-20230915112357-693d4b64813c
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20230915115245-53bd8980dfb7
	github.com/prometheus-community/prom-label-proxy/injectproxy => github.com/prometheus-community/prom-label-proxy/injectproxy v0.6.0
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring => github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.53.1
	github.com/prometheus/common => github.com/prometheus/common v0.37.1
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.40.1
	golang.org/x/net => golang.org/x/net v0.17.0
	k8s.io/api => k8s.io/api v0.26.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.26.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.26.4
	k8s.io/apiserver => k8s.io/apiserver v0.26.4
	k8s.io/client-go => k8s.io/client-go v0.26.4
	k8s.io/component-base => k8s.io/component-base v0.26.4
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.100.1
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280
	sigs.k8s.io/cluster-api => sigs.k8s.io/cluster-api v1.4.7
	sigs.k8s.io/cluster-api-provider-aws/v2 => sigs.k8s.io/cluster-api-provider-aws/v2 v2.2.2
	sigs.k8s.io/cluster-api-provider-ibmcloud => sigs.k8s.io/cluster-api-provider-ibmcloud v0.5.2
	sigs.k8s.io/cluster-api-provider-kubevirt => github.com/openshift/cluster-api-provider-kubevirt v0.0.0-20230126155822-4786167d51b3
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.14.4
)

// needed because otherwise installer fetches a library-go version that requires bitbucket.com/ww/goautoneg which is dead
// Tagged version fetches github.com/munnerz/goautoneg instead
replace github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20230120214501-9bc305884fcb
