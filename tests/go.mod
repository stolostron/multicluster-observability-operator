module github.com/open-cluster-management/observability-e2e-test

go 1.14

require (
	github.com/ghodss/yaml v1.0.0
	github.com/onsi/ginkgo v1.16.1
	github.com/onsi/gomega v1.10.1
	github.com/prometheus/common v0.4.1
	github.com/sclevine/agouti v3.0.0+incompatible
	github.com/stretchr/testify v1.6.1
	golang.org/x/sys v0.0.0-20210414055047-fe65e336abe0 // indirect
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.17.2
	k8s.io/apiextensions-apiserver v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
	k8s.io/klog v1.0.0
	sigs.k8s.io/kustomize/api v0.6.5
	sigs.k8s.io/yaml v1.2.0
)
