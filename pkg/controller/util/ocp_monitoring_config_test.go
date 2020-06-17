// Copyright (c) 2020 Red Hat, Inc.

package util

import (
	"strings"
	"testing"

	monv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetConfigMap(t *testing.T) {
	client := fake.NewSimpleClientset()

	result, err := getConfigMap(client)
	if result != nil {
		t.Errorf("result (%v) is not the expected (nil)", result)
	}

	if !errors.IsNotFound(err) {
		t.Errorf("err (%v) should be (IsNotFound)", err)
	}

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: cmNamespace,
		},
		Data: map[string]string{},
	}

	_, err = client.CoreV1().ConfigMaps(cmNamespace).Create(cm)
	if err != nil {
		t.Errorf("err (%v) is not the expected (nil)", err)
	}

	_, err = getConfigMap(client)
	if errors.IsNotFound(err) {
		t.Errorf("should has a configmap (%v) in namespace (%v), err: (%v)", cmName, cmNamespace, err)
	}
}

func TestCreateRemoteWriteSpec(t *testing.T) {
	labelConfigs := []monv1.RelabelConfig{}
	result, err := createRemoteWriteSpec("testURL", "testClusterID", &labelConfigs)

	if err != nil {
		t.Errorf("err (%v) is not the expected (nil)", err)
	}

	if !strings.HasPrefix(result.URL, protocol) {
		t.Errorf("URL (%v) should has a <%v> prefix", result.URL, protocol)
	}

	if !strings.HasSuffix(result.URL, urlSubPath) {
		t.Errorf("URL (%v) should has a <%v> suffix", result.URL, urlSubPath)
	}

	if !strings.Contains(result.URL, "testURL") {
		t.Errorf("URL (%v) should contains <testURL>", result.URL)
	}

	if result.WriteRelabelConfigs[len(result.WriteRelabelConfigs)-1].TargetLabel != clusterIDLabelKey {
		t.Errorf("replacement (%v) should be equal to <%v>",
			result.WriteRelabelConfigs[len(result.WriteRelabelConfigs)-1].TargetLabel,
			clusterIDLabelKey,
		)
	}

	if result.WriteRelabelConfigs[len(result.WriteRelabelConfigs)-1].Replacement != "testClusterID" {
		t.Errorf("replacement (%v) should be equal to <testClusterID>",
			result.WriteRelabelConfigs[len(result.WriteRelabelConfigs)-1].Replacement)
	}

	result, err = createRemoteWriteSpec(protocol+"testURL"+urlSubPath, "testClusterID", &labelConfigs)
	if !strings.HasPrefix(result.URL, protocol) {
		t.Errorf("URL (%v) should has a <%v> prefix", result.URL, protocol)
	}

	if !strings.HasSuffix(result.URL, urlSubPath) {
		t.Errorf("URL (%v) should has a <%v> suffix", result.URL, urlSubPath)
	}
}
