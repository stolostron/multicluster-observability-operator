module github.com/open-cluster-management/grafana-dashboard-loader

go 1.16

require (
	github.com/spf13/pflag v1.0.5
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/client-go v0.19.4
	k8s.io/klog v1.0.0
)

// Resolves CVE-2020-14040
replace golang.org/x/text => golang.org/x/text v0.3.5
