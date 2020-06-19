// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	"testing"

	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clientv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestCreateKubeSecret(t *testing.T) {
	secretName := "test-secret"
	token := "test-token"
	ca := "test-ca"

	s := scheme.Scheme
	if err := ocinfrav1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add ocinfrav1 scheme: (%v)", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeServiceAccountToken,
		Data: map[string][]byte{
			"token":  []byte(token),
			"ca.crt": []byte(ca),
		},
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
			Annotations: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Kind:      "Secret",
				Namespace: namespace,
				Name:      secretName,
			},
		},
	}
	infra := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "test-api-url",
		},
	}
	objs := []runtime.Object{secret, sa, infra}
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
