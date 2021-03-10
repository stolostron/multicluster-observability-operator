module github.com/open-cluster-management/multicluster-observability-operator

go 1.15

require (
	github.com/Azure/go-autorest/autorest v0.11.6 // indirect
	github.com/go-logr/logr v0.3.0
	github.com/jetstack/cert-manager v0.0.0-00010101000000-000000000000
	github.com/kr/pretty v0.2.1 // indirect
	github.com/open-cluster-management/api v0.0.0-20201007180356-41d07eee4294
	github.com/open-cluster-management/multicloud-operators-placementrule v0.0.0-20201013190828-d760a392d21d
	github.com/open-cluster-management/observatorium-operator v0.0.0-20210208020738-a753d89f15df
	github.com/openshift/api v3.9.1-0.20190424152011-77b8897ec79a+incompatible
	github.com/openshift/client-go v0.0.0-20201020082437-7737f16e53fc
	golang.org/x/tools v0.0.0-20201014231627-1610a49f37af // indirect
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.20.4
	k8s.io/apiextensions-apiserver v0.20.1
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kubectl v0.20.4
	sigs.k8s.io/controller-runtime v0.8.0
	sigs.k8s.io/kustomize/v3 v3.3.1
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/go-logr/logr => github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.2.0
	github.com/jetstack/cert-manager => github.com/open-cluster-management/cert-manager v0.0.0-20200821135248-2fd523b053f5
	github.com/open-cluster-management/api => github.com/open-cluster-management/api v0.0.0-20201007180356-41d07eee4294
	github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2
)
