module github.com/open-cluster-management/multicluster-monitoring-operator

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v40.1.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.10.0 // indirect
	github.com/caddyserver/caddy/v2 v2.1.1 // indirect
	github.com/coreos/prometheus-operator v0.38.0
	github.com/go-openapi/strfmt v0.19.4 // indirect
	github.com/gophercloud/gophercloud v0.8.0 // indirect
	github.com/hashicorp/consul v1.7.4 // indirect
	github.com/jetstack/cert-manager v0.0.0-00010101000000-000000000000
	github.com/observatorium/deployments/operator v0.0.0-20200904140422-ff3b6890e05f
	github.com/open-cluster-management/api v0.0.0-20200602195039-a516cac2e038
	github.com/open-cluster-management/multicloud-operators-placementrule v1.0.0-2020-05-08-20-30-09
	github.com/openshift/api v3.9.1-0.20190424152011-77b8897ec79a+incompatible
	github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
	github.com/openshift/cluster-monitoring-operator v0.1.1-0.20200406225945-844f960a2a8d
	github.com/openshift/origin v1.2.0 // indirect
	github.com/operator-framework/operator-sdk v0.17.0
	github.com/prometheus/prometheus v2.9.2+incompatible // indirect
	github.com/spf13/pflag v1.0.5
	github.com/syndtr/goleveldb v1.0.0 // indirect
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200121175148-a6ecf24a6d71 // indirect
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kubernetes v1.18.8 // indirect
	sigs.k8s.io/controller-runtime v0.5.2
	sigs.k8s.io/kustomize/v3 v3.3.1
	sigs.k8s.io/yaml v1.2.0
)

// Pinned to kubernetes-1.16.2
replace (
	k8s.io/api => k8s.io/api v0.0.0-20191016110408-35e52d86657a
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20191016113550-5357c4baaf65
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191004115801-a2eda9f80ab8
	k8s.io/apiserver => k8s.io/apiserver v0.0.0-20191016112112-5190913f932d
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.0.0-20191016114015-74ad18325ed5
	k8s.io/client-go => k8s.io/client-go v0.0.0-20191016111102-bec269661e48
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.0.0-20191016115326-20453efc2458
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.0.0-20191016115129-c07a134afb42
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20191004115455-8e001e5d1894
	k8s.io/component-base => k8s.io/component-base v0.0.0-20191016111319-039242c015a9
	k8s.io/cri-api => k8s.io/cri-api v0.0.0-20190828162817-608eb1dad4ac
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.0.0-20191016115521-756ffa5af0bd
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.0.0-20191016112429-9587704a8ad4
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.0.0-20191016114939-2b2b218dc1df
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.0.0-20191016114407-2e83b6f20229
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.0.0-20191016114748-65049c67a58b
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20191016120415-2ed914427d51
	k8s.io/kubelet => k8s.io/kubelet v0.0.0-20191016114556-7841ed97f1b2
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.0.0-20191016115753-cf0698c3a16b
	k8s.io/metrics => k8s.io/metrics v0.0.0-20191016113814-3b1a734dba6e
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.0.0-20191016112829-06bb3c9d77c9
)

replace (
	github.com/coreos/etcd => go.etcd.io/etcd v3.3.22+incompatible
	github.com/gorilla/websocket => github.com/gorilla/websocket v1.4.2
	github.com/jetstack/cert-manager => github.com/open-cluster-management/cert-manager v0.0.0-20200821135248-2fd523b053f5
	github.com/mholt/caddy => github.com/caddyserver/caddy v1.0.5
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.0-rc7
	github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190424153033-d3245f150225

)
