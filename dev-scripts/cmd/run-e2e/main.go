// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

// run-e2e generates tests/resources/options.yaml from the live cluster state and
// runs the e2e suite via ginkgo.
//
// It connects to the hub, lists ManagedCluster resources, and matches each one to a
// kubecontext by comparing API server URLs. Reachable clusters are written into
// options.yaml so the test suite can target them for addon tests.
//
// Usage:
//
//	go run ./dev-scripts/cmd/run-e2e [flags]
//
// Flags:
//
//	--hub <context>  kubecontext to use as hub (default: current context)
//	--focus <label>  ginkgo focus pattern (repeatable)
//	--skip  <label>  ginkgo skip pattern (repeatable)
//	--install        let the suite install MCO (default: skipped — MCO assumed deployed)
//	--uninstall      let the suite uninstall MCO after tests (default: skipped)
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Test directory paths relative to the repo root.
const (
	testResourcesDir = "tests/resources"
	testSuiteDir     = "tests/pkg/tests"
)

// Client-side throttling limits for the hub dynamic client.
// The concurrent discovery goroutines share a single client; the default
// QPS=5/Burst=10 causes unnecessary throttling on hubs with many managed clusters.
const (
	hubClientQPS   = 50
	hubClientBurst = 100
)

// stringSlice is a repeatable string flag (--flag a --flag b → ["a","b"]).
type stringSlice []string

func (s *stringSlice) String() string     { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error { *s = append(*s, v); return nil }

const optionsTmpl = `options:
  kubeconfig: {{.Hub.Kubeconfig}}
  hub:
    clusterServerURL: {{.Hub.ServerURL}}
    kubeconfig: {{.Hub.Kubeconfig}}
    kubecontext: {{.Hub.Context}}
    baseDomain: {{.Hub.BaseDomain}}
  clusters:
{{- range .Clusters}}
    - name: {{.Name}}
      clusterServerURL: {{.ServerURL}}
      baseDomain: {{.BaseDomain}}
      kubeconfig: {{.Kubeconfig}}
      kubecontext: {{.Context}}
{{- end}}
`

type clusterInfo struct {
	Name       string
	ServerURL  string
	BaseDomain string
	Kubeconfig string
	Context    string
}

type optionsData struct {
	Hub      clusterInfo
	Clusters []clusterInfo
}

func main() {
	var (
		hubContext string
		focuses    stringSlice
		skips      stringSlice
		install    bool
		uninstall  bool
	)
	flag.StringVar(&hubContext, "hub", "", "kubecontext to use as hub (default: current context)")
	flag.Var(&focuses, "focus", "ginkgo focus pattern (repeatable)")
	flag.Var(&skips, "skip", "ginkgo skip pattern (repeatable)")
	flag.BoolVar(&install, "install", false, "let the suite install MCO (default: skip)")
	flag.BoolVar(&uninstall, "uninstall", false, "let the suite uninstall MCO after tests (default: skip)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, hubContext, focuses, skips, install, uninstall); err != nil {
		slog.Error("fatal", "err", err)
		// Use stop() explicitly before returning so the deferred call is a no-op,
		// allowing the exit code to be communicated via os.Exit without suppressing it.
		stop()
		os.Exit(1) //nolint:gocritic // exitAfterDefer: stop() is called explicitly above
	}
}

func run(ctx context.Context, hubContext string, focuses, skips stringSlice, install, uninstall bool) error {
	if _, err := exec.LookPath("ginkgo"); err != nil {
		return fmt.Errorf("ginkgo not found in PATH — install with: go install github.com/onsi/ginkgo/v2/ginkgo@v2.27.2")
	}

	rawConfig, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return fmt.Errorf("loading kubeconfig: %w", err)
	}
	if hubContext == "" {
		hubContext = rawConfig.CurrentContext
		slog.Info("no --hub specified, using current context", "context", hubContext)
	}

	hubRestConfig, err := restConfigForContext(rawConfig, hubContext)
	if err != nil {
		return fmt.Errorf("building hub REST config: %w", err)
	}
	hubRestConfig.QPS = hubClientQPS
	hubRestConfig.Burst = hubClientBurst

	slog.Info("verifying hub connectivity", "context", hubContext)
	if err := verifyConnectivity(hubRestConfig); err != nil {
		return fmt.Errorf("hub cluster unreachable (context %q): %w", hubContext, err)
	}

	hubServerURL, err := serverURLForContext(rawConfig, hubContext)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(hubRestConfig)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	hubBaseDomain, err := getBaseDomain(ctx, dynamicClient)
	if err != nil {
		return fmt.Errorf("getting hub base domain: %w", err)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return err
	}
	kubeconfigDir := filepath.Join(repoRoot, testResourcesDir, "kubeconfigs")

	hubKubeconfigPath := filepath.Join(kubeconfigDir, "hub.yaml")
	if err := writeContextKubeconfig(rawConfig, hubContext, hubKubeconfigPath); err != nil {
		return err
	}

	hub := clusterInfo{
		ServerURL:  hubServerURL,
		BaseDomain: hubBaseDomain,
		Kubeconfig: hubKubeconfigPath,
		Context:    hubContext,
	}

	clusters, err := discoverClusters(ctx, dynamicClient, rawConfig, kubeconfigDir,
		func(cfg *rest.Config) (dynamic.Interface, error) { return dynamic.NewForConfig(cfg) })
	if err != nil {
		return fmt.Errorf("discovering managed clusters: %w", err)
	}
	if len(clusters) == 0 {
		slog.Info("no accessible managed clusters found, falling back to local-cluster")
		clusters = []clusterInfo{{
			Name:       "local-cluster",
			ServerURL:  hubServerURL,
			BaseDomain: hubBaseDomain,
			Kubeconfig: hubKubeconfigPath,
			Context:    hubContext,
		}}
	}

	optionsFile := filepath.Join(repoRoot, testResourcesDir, "options.yaml")
	if err := writeOptionsFile(optionsFile, optionsData{Hub: hub, Clusters: clusters}); err != nil {
		return err
	}
	slog.Info("options.yaml written", "path", optionsFile)

	return runGinkgo(ctx, repoRoot, optionsFile, focuses, skips, install, uninstall)
}

// discoverClusters lists ManagedClusters on the hub and matches each to a kubecontext
// by comparing API server URLs. Clusters are processed concurrently; unreachable or
// unmatched clusters are skipped with a warning.
// newDynamicClient is injected to allow testing without a live cluster.
func discoverClusters(
	ctx context.Context,
	dynamicClient dynamic.Interface,
	rawConfig *clientcmdapi.Config,
	kubeconfigDir string,
	newDynamicClient func(*rest.Config) (dynamic.Interface, error),
) ([]clusterInfo, error) {
	mcGVR := schema.GroupVersionResource{
		Group:    "cluster.open-cluster-management.io",
		Version:  "v1",
		Resource: "managedclusters",
	}
	mcList, err := dynamicClient.Resource(mcGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing ManagedClusters: %w", err)
	}

	type result struct {
		index   int
		cluster *clusterInfo // nil means skip
		err     error        // non-nil means fatal
	}

	// Limit concurrency to avoid exhausting file descriptors or network connections
	// in large fleets. A buffered channel acts as a counting semaphore.
	const maxConcurrency = 20
	sem := make(chan struct{}, maxConcurrency)

	ch := make(chan result, len(mcList.Items))
	var wg sync.WaitGroup

	for i, mc := range mcList.Items {
		wg.Add(1)
		go func(i int, obj map[string]any) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			name, _ := obj["metadata"].(map[string]any)["name"].(string)

			serverURLs := managedClusterURLs(name, obj)
			if len(serverURLs) == 0 {
				slog.Warn("managed cluster has no client configs, skipping", "cluster", name)
				ch <- result{index: i}
				return
			}

			contextName, serverURL, ok := findContextForURLs(rawConfig, serverURLs)
			if !ok {
				slog.Warn("no matching kubecontext found for managed cluster, skipping",
					"cluster", name, "urls", serverURLs)
				ch <- result{index: i}
				return
			}

			spokeCfg, err := restConfigForContext(rawConfig, contextName)
			if err != nil {
				slog.Warn("cannot build REST config for managed cluster, skipping",
					"cluster", name, "context", contextName, "err", err)
				ch <- result{index: i}
				return
			}

			slog.Info("verifying managed cluster connectivity", "cluster", name, "context", contextName)
			if err := verifyConnectivity(spokeCfg); err != nil {
				slog.Warn("managed cluster unreachable, skipping",
					"cluster", name, "context", contextName, "err", err)
				ch <- result{index: i}
				return
			}

			spokeDynClient, err := newDynamicClient(spokeCfg)
			if err != nil {
				ch <- result{index: i, err: fmt.Errorf("creating dynamic client for %q: %w", name, err)}
				return
			}
			baseDomain, err := getBaseDomain(ctx, spokeDynClient)
			if err != nil {
				baseDomain = baseDomainFromURL(serverURL)
				slog.Warn("could not get ingress domain, using derived value",
					"cluster", name, "baseDomain", baseDomain, "err", err)
			}

			kubeconfigPath := filepath.Join(kubeconfigDir, fmt.Sprintf("spoke-%s.yaml", name))
			if err := writeContextKubeconfig(rawConfig, contextName, kubeconfigPath); err != nil {
				ch <- result{index: i, err: err}
				return
			}

			slog.Info("managed cluster matched", "cluster", name, "context", contextName, "baseDomain", baseDomain)
			ch <- result{index: i, cluster: &clusterInfo{
				Name:       name,
				ServerURL:  serverURL,
				BaseDomain: baseDomain,
				Kubeconfig: kubeconfigPath,
				Context:    contextName,
			}}
		}(i, mc.Object)
	}

	wg.Wait()
	close(ch)

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("cluster discovery interrupted: %w", err)
	}

	// Collect results, preserving insertion order.
	ordered := make([]*clusterInfo, len(mcList.Items))
	for r := range ch {
		if r.err != nil {
			return nil, r.err
		}
		ordered[r.index] = r.cluster
	}

	var clusters []clusterInfo
	for _, c := range ordered {
		if c != nil {
			clusters = append(clusters, *c)
		}
	}
	return clusters, nil
}

// managedClusterURLs extracts API server URLs from a ManagedCluster's unstructured object.
func managedClusterURLs(name string, obj map[string]any) []string {
	spec, ok := obj["spec"].(map[string]any)
	if !ok {
		slog.Warn("managed cluster spec has unexpected type or is missing", "cluster", name)
		return nil
	}
	configs, ok := spec["managedClusterClientConfigs"].([]any)
	if !ok {
		slog.Warn("managed cluster managedClusterClientConfigs has unexpected type or is missing", "cluster", name)
		return nil
	}
	var urls []string
	for _, cfg := range configs {
		m, ok := cfg.(map[string]any)
		if !ok {
			slog.Warn("managed cluster client config entry has unexpected type", "cluster", name)
			continue
		}
		u, ok := m["url"].(string)
		if ok && u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

// findContextForURLs returns the first kubecontext whose cluster server URL matches
// one of the provided URLs. Comparison is case-insensitive with trailing slashes stripped.
func findContextForURLs(rawConfig *clientcmdapi.Config, serverURLs []string) (contextName, serverURL string, ok bool) {
	normalize := func(u string) string { return strings.TrimRight(strings.ToLower(u), "/") }

	// Build a set of normalized target URLs.
	targetSet := make(map[string]string, len(serverURLs))
	for _, u := range serverURLs {
		targetSet[normalize(u)] = u
	}

	// Find kubeconfig cluster entries whose server matches one of the targets.
	matchingClusters := map[string]string{} // kubeconfig cluster name → original server URL
	for clusterName, cluster := range rawConfig.Clusters {
		if orig, matched := targetSet[normalize(cluster.Server)]; matched {
			matchingClusters[clusterName] = orig
		}
	}

	// Collect all contexts that reference a matching cluster, then sort by name
	// so the selection is deterministic regardless of map iteration order.
	type match struct {
		ctxName   string
		serverURL string
	}
	var matches []match
	for ctxName, ctx := range rawConfig.Contexts {
		if orig, matched := matchingClusters[ctx.Cluster]; matched {
			matches = append(matches, match{ctxName, orig})
		}
	}
	if len(matches) == 0 {
		return "", "", false
	}
	slices.SortFunc(matches, func(a, b match) int { return strings.Compare(a.ctxName, b.ctxName) })
	return matches[0].ctxName, matches[0].serverURL, true
}

// getBaseDomain returns the cluster base domain by reading the default IngressController.
// The IngressController is the authoritative source on OCP: it owns the wildcard DNS record
// (*.apps.<domain>) and exposes the full ingress domain as status.domain. The API server URL
// uses a separate "api." subdomain that may differ from the ingress domain in some topologies
// (e.g. SNO, HCP), so we prefer this source over parsing the server URL.
func getBaseDomain(ctx context.Context, dynamicClient dynamic.Interface) (string, error) {
	ingCtrlGVR := schema.GroupVersionResource{
		Group:    "operator.openshift.io",
		Version:  "v1",
		Resource: "ingresscontrollers",
	}
	ingCtrl, err := dynamicClient.Resource(ingCtrlGVR).
		Namespace("openshift-ingress-operator").
		Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting IngressController: %w", err)
	}
	status, _ := ingCtrl.Object["status"].(map[string]any)
	domain, _ := status["domain"].(string)
	if domain == "" {
		return "", fmt.Errorf("IngressController status.domain is empty")
	}
	return strings.TrimPrefix(domain, "apps."), nil
}

// baseDomainFromURL derives a base domain from an API server URL as a fallback.
// https://api.cluster.example.com:6443 → cluster.example.com
func baseDomainFromURL(serverURL string) string {
	u := strings.TrimPrefix(serverURL, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "api.")
	return strings.SplitN(u, ":", 2)[0]
}

// contextFromConfig looks up a named context in the kubeconfig, returning a clear error if absent.
func contextFromConfig(rawConfig *clientcmdapi.Config, contextName string) (*clientcmdapi.Context, error) {
	ctx, ok := rawConfig.Contexts[contextName]
	if !ok {
		return nil, fmt.Errorf("context %q not found in kubeconfig", contextName)
	}
	return ctx, nil
}

func restConfigForContext(rawConfig *clientcmdapi.Config, contextName string) (*rest.Config, error) {
	cfg, err := clientcmd.NewNonInteractiveClientConfig(
		*rawConfig, contextName, &clientcmd.ConfigOverrides{}, nil,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building REST config for context %q: %w", contextName, err)
	}
	return cfg, nil
}

func serverURLForContext(rawConfig *clientcmdapi.Config, contextName string) (string, error) {
	ctx, err := contextFromConfig(rawConfig, contextName)
	if err != nil {
		return "", err
	}
	cluster, ok := rawConfig.Clusters[ctx.Cluster]
	if !ok {
		return "", fmt.Errorf("cluster %q not found for context %q", ctx.Cluster, contextName)
	}
	return cluster.Server, nil
}

func verifyConnectivity(restConfig *rest.Config) error {
	cfg := rest.CopyConfig(restConfig)
	cfg.Timeout = 10 * time.Second
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("creating discovery client: %w", err)
	}
	if _, err := dc.ServerVersion(); err != nil {
		return fmt.Errorf("server version check failed: %w", err)
	}
	return nil
}

// writeContextKubeconfig writes a single-context kubeconfig to destPath.
func writeContextKubeconfig(rawConfig *clientcmdapi.Config, contextName, destPath string) error {
	ctx, err := contextFromConfig(rawConfig, contextName)
	if err != nil {
		return err
	}
	minCfg := clientcmdapi.NewConfig()
	minCfg.CurrentContext = contextName
	minCfg.Contexts[contextName] = ctx
	if cluster, ok := rawConfig.Clusters[ctx.Cluster]; ok {
		minCfg.Clusters[ctx.Cluster] = cluster
	}
	if authInfo, ok := rawConfig.AuthInfos[ctx.AuthInfo]; ok {
		minCfg.AuthInfos[ctx.AuthInfo] = authInfo
	}
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating kubeconfig directory %s: %w", dir, err)
	}
	if err := clientcmd.WriteToFile(*minCfg, destPath); err != nil {
		return fmt.Errorf("writing kubeconfig to %s: %w", destPath, err)
	}
	return nil
}

func writeOptionsFile(path string, data optionsData) error {
	tmpl, err := template.New("options").Parse(optionsTmpl)
	if err != nil {
		return fmt.Errorf("parsing options template: %w", err)
	}
	f, err := os.Create(path) //nolint:gosec // path is an internal repo-relative location, not user input
	if err != nil {
		return fmt.Errorf("creating options file %s: %w", path, err)
	}
	if err := tmpl.Execute(f, data); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			slog.Warn("closing options file after render failure", "path", path, "err", closeErr)
		}
		return fmt.Errorf("rendering options template: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing options file %s: %w", path, err)
	}
	return nil
}

// findRepoRoot walks up from the working directory to the directory containing go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repo root not found (no go.mod in any parent directory)")
		}
		dir = parent
	}
}

func runGinkgo(ctx context.Context, repoRoot, optionsFile string, focuses, skips stringSlice, install, uninstall bool) error {
	args := []string{
		"--no-color",
		"--junit-report=" + filepath.Join(repoRoot, testSuiteDir, "results.xml"),
		"-debug", "-trace", "-v",
	}

	appendFlag := func(flag string, vals stringSlice) {
		for _, v := range vals {
			args = append(args, flag, v)
		}
	}
	appendFlag("--focus", focuses)
	appendFlag("--skip", skips)

	args = append(args,
		filepath.Join(repoRoot, testSuiteDir),
		"--",
		"-options="+optionsFile,
		"-v=2",
	)

	cmd := exec.CommandContext(ctx, "ginkgo", args...) //nolint:gosec // ginkgo is a known test runner resolved via PATH
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("SKIP_INSTALL_STEP=%v", !install),
		fmt.Sprintf("SKIP_UNINSTALL_STEP=%v", !uninstall),
	)

	slog.Info("running ginkgo", "args", strings.Join(args, " "))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ginkgo: %w", err)
	}
	return nil
}
