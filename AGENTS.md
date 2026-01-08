# AGENTS.md - Context & Operating System for AI Agents

> **Role:** You are an expert Software Engineer and Kubernetes Operator specialist working on the Multi-Cluster Observability Operator (MCO). You are responsible for maintaining the legacy observability stack while assisting with the transition to the new Multi-Cluster Observability Addon (MCOA) architecture.

## 1. Project Architecture & Domain

### 1.1 Overview
MCO is an ACM (Advanced Cluster Management) addon that provides unified observability (metrics, alerts) across a fleet of managed clusters. It orchestrates a complete monitoring stack based on Thanos, Prometheus, Alertmanager, and Grafana.

### 1.2 Core Architectures
The project is currently in a transitional state between two architectures:

#### A. Legacy Architecture (Deprecating)
*   **Mechanism:** `observability-endpoint-operator` is deployed on managed clusters via `ManifestWorks`.
*   **Data Flow:** `metrics-collector` (custom code) federates from in-cluster Prometheus -> mTLS -> Hub (Thanos Receive).
*   **Key Components:**
    *   `operators/endpointmetrics`: The controller running on managed clusters.
    *   `collectors/metrics`: The custom scraper/forwarder.
    *   `metrics-allowlist`: ConfigMap-based configuration (`operators/multiclusterobservability/manifests/base/config/metrics_allowlist.yaml`).
    *   `operators/multiclusterobservability/controllers/placementrule`: Acts as a custom addon manager for the legacy architecture (historical quirk).

#### B. MCOA Architecture (New Standard)
*   **Mechanism:** Fully leverages the `addon-framework` and upstream OCM APIs (`Placement`, `ClusterManagementAddon`) for deployment and management.
*   **Data Flow:** `PrometheusAgent` (upstream `monitoring.rhobs` API) -> Remote Write -> Hub.
*   **Key Components:**
    *   `PrometheusAgent`: Replaces the metrics collector.
    *   `ScrapeConfig` & `PrometheusRule`: Standard APIs for configuration.

### 1.3 The Global Hub & Hub Self-Management Constraints
*   **Hub Self-Management:** The `hubselfmanagement` configuration in MCH typically controls whether the hub is treated as a managed cluster. Normally, disabling this removes addons. However, **metrics collection on the hub must persist** regardless of this setting. We enforce this presence to ensure continuous observability of the hub itself.
*   **Direct Deployment:** As a consequence, resources for the hub are **not** deployed using `ManifestWorks` and the `workAgent` (which respect `hubselfmanagement`). Instead, they are deployed directly using the MCO operator's **controller client**.
*   **Multi-Parent Support:** In a Global Hub architecture, a specific "managed hub" can be imported by multiple parent hubs. To support this, the architecture allows running **multiple metrics collectors** on a single hub—one dedicated to each parent hub relationship.
*   **Hub ID:** Alert forwarding secrets are suffixed with the target hub ID to differentiate between these multiple parent connections.

## 2. Directory Map & Context Boundaries

| Path | Context | Description |
| :--- | :--- | :--- |
| `operators/multiclusterobservability` | **High** | The "Root" operator (Hub side). Manages the Thanos stack and MCOA orchestration. |
| `operators/endpointmetrics` | **Medium** | Legacy managed cluster operator. |
| `collectors/metrics` | **Medium** | Legacy custom metrics collector code. |
| `proxy/` | **Medium** | RBAC Query Proxy. Enforces ACM permissions on metric queries. |
| `loaders/dashboards` | **Low** | Sidecar for loading Grafana dashboards. |
| `manifests/` | **High** | Kustomize bases and raw manifests. |
| `tests/` | **High** | E2E and integration tests. |
| `cicd-scripts/` | **Low** | Prow/Jenkins CI scripts. |

### 2.1 Context Exclusion (Ignore these)
> **Do not index or read:** `vendor/`, `tmp/`, `.git/`, `*.log`, `bin/`, `coverage/`.

## 3. Development Protocols

### 3.1 Build & Run
*   **Discovery:** Run `make help` to see all available make targets with descriptions.
*   **Build Binary:** `make build` (Targets `operators/multiclusterobservability`).
*   **Build Images:** `make docker-build` (requires auth) or `make docker-build-local`.
*   **Format:** `make format` (Strict requirement).
*   **Lint:**
    *   `make lint`: Applies all checks (format, deps, copyright, golangci-lint). **Note:** This command requires a clean work tree and will fail if there are unstaged or uncommitted changes.
    *   `make go-lint`: Runs only `golangci-lint` on Go code. Use this for faster iteration or when you have staged changes that would cause `make lint` to fail.

### 3.2 Testing Strategy
*   **Unit Tests:** `make unit-tests`. **Primary verification method.** Use for all logic verification.
*   **Integration:** `make integration-test`. Run these to verify controller interactions.
*   **E2E (Kind):** **Exceptional Workflow.** Running full E2E tests locally (`make mco-kind-env`, `make e2e-tests-in-kind`) is resource-intensive and rarely needed for routine tasks. Rely on CI for full system validation.

### 3.3 Coding Standards (Go)
*   **Version:** Refer to `go.mod` for the current Go version and toolchain.
*   **Commenting:** Focus on the "Why" over the "What". Add comments for non-obvious logic, architectural decisions, or complex workarounds where the rationale isn't immediately clear from the code itself.
*   **Error Handling:** Use `fmt.Errorf("...: %w", err)` for wrapping. Handle errors once (log OR return, never both).
*   **Logging:** Use structured logging (`log.Info("msg", "key", value)`). Log all state changes (Create/Update/Delete).
*   **Kubernetes Patterns:**
    *   **Idempotency:** Reconcile loops must be safe to re-run.
    *   **Status:** Update `Status` subresources independently of `Spec`.
    *   **Client:** Use `client.Reader` for cached reads, `APIReader` only for strong consistency after writes.

### 3.4 Commit Protocol
*   **DCO:** All commits **MUST** be signed off: `Signed-off-by: Name <email@example.com>`.

## 4. Agent Operational Protocol

1.  **PLAN:** Before writing code, analyze the request. Identify if it touches Legacy or MCOA components.
2.  **ACT:** Make changes. If modifying `operators/multiclusterobservability`, ensure backward compatibility.
3.  **REFLECT:**
    *   Did I respect the architectural constraints and specific exceptions (Section 1)?
    *   Did I sign-off the commit?
    *   Run `make format` and `make go-lint`.
