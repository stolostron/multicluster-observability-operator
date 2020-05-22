// Copyright (c) 2020 Red Hat, Inc.

package placementrule

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/open-cluster-management/multicluster-monitoring-operator/pkg/controller/util"
)

const (
	kubeConfigName = "hub-kube-config"
)

func createKubeConfig(client client.Client, namespace string) (*clientv1.Config, error) {
	ca, token, error := getSAToken(client, namespace)
	if error != nil {
		return nil, error
	}
	apiServer, error := util.GetKubeAPIServerAddress(client)
	if error != nil {
		return nil, error
	}
	return &clientv1.Config{
		Kind:       "Config",
		APIVersion: "v1",
		Clusters: []clientv1.NamedCluster{
			{
				Name: "default-cluster",
				Cluster: clientv1.Cluster{
					Server:                   apiServer,
					CertificateAuthorityData: ca,
				},
			},
		},
		AuthInfos: []clientv1.NamedAuthInfo{
			{
				Name: "default-user",
				AuthInfo: clientv1.AuthInfo{
					Token: string(token),
				},
			},
		},
		Contexts: []clientv1.NamedContext{
			{
				Name: "default-context",
				Context: clientv1.Context{
					Cluster:   "default-cluster",
					AuthInfo:  "default-user",
					Namespace: namespace,
				},
			},
		},
		CurrentContext: "default-context",
	}, nil
}

func createKubeSecret(client client.Client, namespace string) (*corev1.Secret, error) {
	config, err := createKubeConfig(client, namespace)
	if err != nil {
		return nil, err
	}
	configYaml, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubeConfigName,
			Namespace: spokeNameSpace,
		},
		Data: map[string][]byte{
			"config": configYaml,
		},
	}, nil
}
