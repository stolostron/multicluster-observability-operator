// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func newTestInfra() *ocinfrav1.Infrastructure {
	return &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "test-api-url",
		},
	}
}

func TestCreateKubeSecret(t *testing.T) {
	initSchema(t)

	objs := []runtime.Object{newSATokenSecret(), newTestSA(), newTestInfra()}
	c := fake.NewFakeClient(objs...)

	kubeconfig, err := createKubeSecret(c, namespace)
	if err != nil {
		t.Fatalf("Failed to create kubeconfig secret: (%v)", err)
	}
	config := &clientv1.Config{}
	err = yaml.Unmarshal(kubeconfig.Data["config"], config)
	if err != nil {
		t.Fatalf("Failed to unmarshal config in kubeconfig secret: (%v)", err)
	}
	if config.AuthInfos[0].AuthInfo.Token != token {
		t.Fatal("Wrong token included in the kubeconfig")
	}

}
