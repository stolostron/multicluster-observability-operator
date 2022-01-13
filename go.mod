module github.com/stolostron/multicluster-monitoring-operator

go 1.13

require (
	github.com/jetstack/cert-manager v0.0.0-00010101000000-000000000000
	github.com/open-cluster-management/api v0.0.0-20210513122330-d76f10481f05
	github.com/openshift/api v3.9.1-0.20190924102528-32369d4db2ad+incompatible
	github.com/openshift/client-go v0.0.0-20201020082437-7737f16e53fc
	github.com/operator-framework/operator-sdk v0.18.0
	github.com/stolostron/multicloud-operators-placementrule v0.0.0-20220113003548-6d97a1d71270
	github.com/stolostron/observatorium-operator v0.0.0-20220112145421-997624bbce60
	github.com/sykesm/zap-logfmt v0.0.4
	go.uber.org/zap v1.15.0
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/kubectl v0.19.0
	sigs.k8s.io/controller-runtime v0.6.3
	sigs.k8s.io/kustomize/v3 v3.3.1
	sigs.k8s.io/yaml v1.2.0
)

replace (
	bitbucket.org/ww/goautoneg => github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/coreos/etcd => go.etcd.io/etcd v3.3.22+incompatible
	github.com/go-logr/logr => github.com/go-logr/logr v0.2.1
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.2.0
	github.com/gorilla/websocket => github.com/gorilla/websocket v1.4.2
	github.com/jetstack/cert-manager => github.com/stolostron/cert-manager v0.0.0-20200821135248-2fd523b053f5
	github.com/mholt/caddy => github.com/caddyserver/caddy v1.0.5
	github.com/open-cluster-management/api => open-cluster-management.io/api v0.0.0-20201007180356-41d07eee4294
	github.com/opencontainers/runc => github.com/opencontainers/runc v1.0.0-rc7
	github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190424153033-d3245f150225
	k8s.io/api => k8s.io/api v0.19.0
	k8s.io/client-go => k8s.io/client-go v0.19.0
)
