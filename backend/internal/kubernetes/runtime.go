package kubernetes

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	cmodel "github.com/kolapsis/pulseboard/internal/container"
	pbruntime "github.com/kolapsis/pulseboard/internal/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

func init() {
	pbruntime.Register("kubernetes", func(ctx context.Context, logger *slog.Logger) (pbruntime.Runtime, error) {
		allowNS := os.Getenv("PULSEBOARD_K8S_NAMESPACES")
		excludeNS := os.Getenv("PULSEBOARD_K8S_EXCLUDE_NAMESPACES")
		nsFilter := NewNamespaceFilter(allowNS, excludeNS)
		return NewRuntime(logger, nsFilter)
	})
}

// Runtime implements runtime.Runtime for Kubernetes.
type Runtime struct {
	logger    *slog.Logger
	nsFilter  *NamespaceFilter
	clientset k8s.Interface
	metrics   metricsv.Interface
	factory   informers.SharedInformerFactory
	stopCh    chan struct{}

	mu        sync.Mutex
	connected bool
	prevCPU   map[string]*cpuPrev // CPU delta state keyed by externalID
}

type cpuPrev struct {
	milliCPU  int64
	timestamp time.Time
}

// NewRuntime creates a Kubernetes runtime. Connection is deferred to Connect().
func NewRuntime(logger *slog.Logger, nsFilter *NamespaceFilter) (*Runtime, error) {
	return &Runtime{
		logger:   logger,
		nsFilter: nsFilter,
		prevCPU:  make(map[string]*cpuPrev),
		stopCh:   make(chan struct{}),
	}, nil
}

func (r *Runtime) Connect(ctx context.Context) error {
	config, err := buildConfig()
	if err != nil {
		return fmt.Errorf("kubernetes config: %w", err)
	}

	clientset, err := k8s.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("kubernetes clientset: %w", err)
	}

	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		r.logger.Warn("metrics-server client failed; resource metrics will be unavailable", "error", err)
	}

	// Verify connectivity.
	_, err = clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("kubernetes connectivity check failed: %w", err)
	}

	factory := informers.NewSharedInformerFactory(clientset, 30*time.Second)

	r.mu.Lock()
	r.clientset = clientset
	r.metrics = metricsClient
	r.factory = factory
	r.connected = true
	r.mu.Unlock()

	// Start informers.
	factory.Start(r.stopCh)
	factory.WaitForCacheSync(r.stopCh)

	r.logger.Info("kubernetes runtime connected")
	return nil
}

func buildConfig() (*rest.Config, error) {
	// In-cluster first.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		cfg, err := rest.InClusterConfig()
		if err == nil {
			return cfg, nil
		}
	}

	// KUBECONFIG env or default path.
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			kubeconfig = home + "/.kube/config"
		}
	}
	if kubeconfig != "" {
		if _, err := os.Stat(kubeconfig); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfig)
		}
	}

	return nil, fmt.Errorf("no kubernetes config found (not in-cluster, no KUBECONFIG, no ~/.kube/config)")
}

func (r *Runtime) IsConnected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connected
}

func (r *Runtime) SetDisconnected() {
	r.mu.Lock()
	r.connected = false
	r.mu.Unlock()
}

func (r *Runtime) Close() error {
	close(r.stopCh)
	return nil
}

func (r *Runtime) Name() string {
	return "kubernetes"
}

func (r *Runtime) DiscoverAll(ctx context.Context) ([]*cmodel.Container, error) {
	return r.discoverAll(ctx)
}

func (r *Runtime) StreamEvents(ctx context.Context) <-chan pbruntime.RuntimeEvent {
	return r.streamEvents(ctx)
}

func (r *Runtime) StatsSnapshot(ctx context.Context, externalID string) (*pbruntime.RawStats, error) {
	return r.statsSnapshot(ctx, externalID)
}

func (r *Runtime) FetchLogs(ctx context.Context, externalID string, lines int, timestamps bool) ([]string, error) {
	return r.fetchLogs(ctx, externalID, lines, timestamps)
}

func (r *Runtime) StreamLogs(ctx context.Context, externalID string, lines int, timestamps bool) (io.ReadCloser, error) {
	return r.streamLogs(ctx, externalID, lines, timestamps)
}

func (r *Runtime) GetHealthInfo(ctx context.Context, externalID string) (*pbruntime.HealthInfo, error) {
	return r.getHealthInfo(ctx, externalID)
}

// ListContainerNames returns the container names in a workload's pod spec.
// For controllers, resolves to a pod's spec. For bare pods, reads the pod directly.
func (r *Runtime) ListContainerNames(ctx context.Context, externalID string) ([]string, error) {
	ns, podName, _, err := r.resolveLogTarget(ctx, externalID)
	if err != nil {
		return nil, err
	}

	pod, err := r.clientset.CoreV1().Pods(ns).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pod %s/%s: %w", ns, podName, err)
	}

	var names []string
	for _, c := range pod.Spec.InitContainers {
		names = append(names, c.Name+" (init)")
	}
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	return names, nil
}

// FetchLogSnippet retrieves the last 50 lines for die event snippets.
// Satisfies container.LogFetcher interface.
func (r *Runtime) FetchLogSnippet(ctx context.Context, externalID string) (string, error) {
	lines, err := r.fetchLogs(ctx, externalID, 50, false)
	if err != nil {
		return "", err
	}
	result := ""
	for _, l := range lines {
		result += l + "\n"
	}
	return result, nil
}
