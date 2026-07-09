// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

//go:build integration

package util

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	tlsutil "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	ctrlwebhook "sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	testEnv    *envtest.Environment
	restCfg    *rest.Config
	testScheme *runtime.Scheme
)

func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{Development: true})))

	testScheme = runtime.NewScheme()
	configv1.AddToScheme(testScheme)

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:       []string{filepath.Join("testdata", "crd")},
		ControlPlaneStopTimeout: 5 * time.Minute,
	}

	var err error
	restCfg, err = testEnv.Start()
	if err != nil {
		panic(fmt.Sprintf("failed to start envtest: %v", err))
	}

	exitCode := m.Run()

	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("failed to stop envtest: %v", err))
	}

	os.Exit(exitCode)
}

type profileChange struct {
	old configv1.TLSProfileSpec
	new configv1.TLSProfileSpec
}

func TestIntegrationSecurityProfileWatcherOnProfileChange(t *testing.T) {
	defer resetTLSState()

	k8sClient, err := client.New(restCfg, client.Options{Scheme: testScheme})
	require.NoError(t, err)

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}
	require.NoError(t, k8sClient.Create(context.Background(), apiServer))
	defer func() {
		_ = k8sClient.Delete(context.Background(), apiServer)
	}()

	tlsClientFunc = func() (client.Client, error) {
		return k8sClient, nil
	}

	ctx := context.Background()

	// Fetch profile and TLS config via the util functions
	tlsProfileSpec, err := GetOrCreateTLSProfileSpec(ctx)
	require.NoError(t, err)
	tlsConfig, err := GetOrCreateTLSConfig(ctx)
	require.NoError(t, err)

	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme: testScheme,
		Metrics: ctrlmetrics.Options{
			BindAddress: "0",
			TLSOpts:     []func(*tls.Config){tlsConfig},
		},
		WebhookServer: ctrlwebhook.NewServer(ctrlwebhook.Options{
			Port:    0,
			TLSOpts: []func(*tls.Config){tlsConfig},
		}),
		Controller: ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)

	changes := make(chan profileChange, 1)

	watcher := &tlsutil.SecurityProfileWatcher{
		Client:                mgr.GetClient(),
		InitialTLSProfileSpec: *tlsProfileSpec,
		OnProfileChange: func(ctx context.Context, oldSpec, newSpec configv1.TLSProfileSpec) {
			changes <- profileChange{old: oldSpec, new: newSpec}
		},
	}
	require.NoError(t, watcher.SetupWithManager(mgr))

	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	defer mgrCancel()

	go func() {
		if err := mgr.Start(mgrCtx); err != nil {
			t.Logf("manager stopped: %v", err)
		}
	}()

	require.True(t, mgr.GetCache().WaitForCacheSync(mgrCtx))

	// Change profile from Intermediate to Modern
	require.NoError(t, k8sClient.Get(context.Background(), client.ObjectKeyFromObject(apiServer), apiServer))
	apiServer.Spec.TLSSecurityProfile = &configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileModernType,
	}
	require.NoError(t, k8sClient.Update(context.Background(), apiServer))

	select {
	case change := <-changes:
		assert.Equal(t, *tlsProfileSpec, change.old)
		assert.Equal(t, *configv1.TLSProfiles[configv1.TLSProfileModernType], change.new)
	case <-time.After(10 * time.Second):
		t.Fatal("expected a profile change callback")
	}
}
