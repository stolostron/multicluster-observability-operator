module github.com/open-cluster-management/multicluster-observability-operator

go 1.16

require (
	github.com/IBM/controller-filtered-cache v0.3.1
	github.com/cloudflare/cfssl v1.5.0
	github.com/go-logr/logr v0.4.0
	github.com/hashicorp/go-version v1.2.0
	github.com/open-cluster-management/addon-framework v0.0.0-20210519012201-d00a09b436d2
	github.com/open-cluster-management/api v0.0.0-20210409125704-06f2aec1a73f
	github.com/open-cluster-management/multicloud-operators-placementrule v0.0.0-20210325184301-dd3e27fc2978
	github.com/open-cluster-management/observatorium-operator v0.0.0-20210512092645-e959481410d7
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/client-go v0.0.0-20201214125552-e615e336eb49
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.1
	k8s.io/apiextensions-apiserver v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kubectl v0.21.0
	k8s.io/utils v0.0.0-20210527160623-6fdb442a123b
	sigs.k8s.io/controller-runtime v0.9.0
	sigs.k8s.io/kube-storage-version-migrator v0.0.3
	sigs.k8s.io/kustomize/v3 v3.3.1
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/open-cluster-management/api => github.com/open-cluster-management/api v0.0.0-20210409125704-06f2aec1a73f
	github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	golang.org/x/text => golang.org/x/text v0.3.5
	k8s.io/client-go => k8s.io/client-go v0.21.0
	sigs.k8s.io/kube-storage-version-migrator => github.com/openshift/kubernetes-kube-storage-version-migrator v0.0.3-0.20210302135122-481bd04dbc78
)
