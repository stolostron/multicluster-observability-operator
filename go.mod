module github.com/open-cluster-management/multicluster-observability-operator

go 1.15

require (
	github.com/Azure/go-autorest/autorest v0.11.6 // indirect
	github.com/cloudflare/cfssl v1.5.0
	github.com/go-logr/logr v0.4.0
	github.com/jetstack/cert-manager v0.0.0-00010101000000-000000000000
	github.com/kr/pretty v0.2.1 // indirect
	github.com/open-cluster-management/addon-framework v0.0.0-20210414095446-30a5d245b8c7
	github.com/open-cluster-management/api v0.0.0-20210409125704-06f2aec1a73f
	github.com/open-cluster-management/multicloud-operators-placementrule v0.0.0-20210325184301-dd3e27fc2978
	github.com/open-cluster-management/observatorium-operator v0.0.0-20210421123734-13feeb802b22 
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/client-go v0.0.0-20201214125552-e615e336eb49
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.5
	k8s.io/apiextensions-apiserver v0.20.2
	k8s.io/apimachinery v0.20.5
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kubectl v0.20.4
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.8.0
	sigs.k8s.io/kube-storage-version-migrator v0.0.3
	sigs.k8s.io/kustomize/v3 v3.3.1
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/go-logr/logr => github.com/go-logr/logr v0.3.0
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.2.0
	github.com/jetstack/cert-manager => github.com/open-cluster-management/cert-manager v0.0.0-20200821135248-2fd523b053f5
	github.com/open-cluster-management/api => github.com/open-cluster-management/api v0.0.0-20210409125704-06f2aec1a73f
	github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	golang.org/x/text => golang.org/x/text v0.3.5
	k8s.io/api => k8s.io/api v0.19.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.2
	k8s.io/client-go => k8s.io/client-go v0.19.2
	sigs.k8s.io/kube-storage-version-migrator => github.com/openshift/kubernetes-kube-storage-version-migrator v0.0.3-0.20210302135122-481bd04dbc78
)
