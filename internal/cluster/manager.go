package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/utils/log"
)

// ClusterClientManager manages a pool of ClusterClients by kubeconfig context.
type ClusterClientManager struct {
	mu             sync.RWMutex
	kubeconfigPath string
	clients        map[string]*ClusterClient
	healthTick     *time.Ticker
	stopCh         chan struct{}
}

// NewClusterClientManager creates a manager, discovers contexts, and starts health checks.
func NewClusterClientManager(kubeconfigPath string) (*ClusterClientManager, error) {
	if kubeconfigPath == "" {
		kubeconfigPath = clientcmd.RecommendedHomeFile
	}

	m := &ClusterClientManager{
		kubeconfigPath: kubeconfigPath,
		clients:        make(map[string]*ClusterClient),
		stopCh:         make(chan struct{}),
	}

	// Initial discovery — non-fatal; kubeconfig may be fixed later
	if err := m.discover(); err != nil {
		config.L().Warn("initial kubeconfig discovery failed", zap.Error(err))
	}

	m.healthTick = time.NewTicker(15 * time.Second)
	go m.healthLoop()

	return m, nil
}

// discover reads kubeconfig and builds the client map with fingerprints.
func (m *ClusterClientManager) discover() error {
	cfg, err := clientcmd.LoadFromFile(m.kubeconfigPath)
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, ctx := range cfg.Contexts {
		if _, exists := m.clients[name]; exists {
			continue
		}
		fingerprint := ""
		if cluster, ok := cfg.Clusters[ctx.Cluster]; ok && len(cluster.CertificateAuthorityData) > 0 {
			h := sha256.Sum256(cluster.CertificateAuthorityData)
			fingerprint = hex.EncodeToString(h[:])
		}

		m.clients[name] = &ClusterClient{
			ContextName: name,
			Fingerprint: fingerprint,
			Health:      HealthOffline,
		}
	}

	// Remove deleted contexts
	for name := range m.clients {
		if cfg.Contexts[name] == nil {
			delete(m.clients, name)
		}
	}

	return nil
}

// GetClient returns a ClusterClient for the given context name, lazily connecting on first use.
func (m *ClusterClientManager) GetClient(ctx context.Context, name string) (*ClusterClient, error) {
	m.mu.RLock()
	cc, ok := m.clients[name]
	m.mu.RUnlock()
	if !ok {
		log.Ctx(ctx).Error("cluster client failed",
			zap.String("event", "cluster.error"),
			zap.String("cluster", name),
			zap.Error(fmt.Errorf("cluster %q not found in kubeconfig", name)),
		)
		return nil, fmt.Errorf("cluster %q not found in kubeconfig", name)
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cc.clientset != nil {
		log.Ctx(ctx).Info("cluster client obtained",
			zap.String("event", "cluster.connected"),
			zap.String("cluster", name),
		)
		return cc, nil
	}

	// Build rest config targeting this specific context
	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: m.kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: name},
	).ClientConfig()
	if err != nil {
		cc.Health = HealthOffline
		log.Ctx(ctx).Error("cluster client failed",
			zap.String("event", "cluster.error"),
			zap.String("cluster", name),
			zap.Error(fmt.Errorf("build config for %s: %w", name, err)),
		)
		return nil, fmt.Errorf("build config for %s: %w", name, err)
	}

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		cc.Health = HealthOffline
		log.Ctx(ctx).Error("cluster client failed",
			zap.String("event", "cluster.error"),
			zap.String("cluster", name),
			zap.Error(err),
		)
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		cc.Health = HealthOffline
		log.Ctx(ctx).Error("cluster client failed",
			zap.String("event", "cluster.error"),
			zap.String("cluster", name),
			zap.Error(err),
		)
		return nil, err
	}

	cc.clientset = clientset
	cc.dynamic = dynamicClient
	cc.restConfig = restCfg
	cc.Health = HealthHealthy
	cc.LastSeen = time.Now()

	log.Ctx(ctx).Info("cluster client obtained",
		zap.String("event", "cluster.connected"),
		zap.String("cluster", name),
	)

	return cc, nil
}

// ListClusters returns summaries for all known clusters by polling in parallel.
func (m *ClusterClientManager) ListClusters(ctx context.Context) []ClusterSummary {
	log.Ctx(ctx).Debug("listing clusters")
	m.mu.RLock()
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	m.mu.RUnlock()

	results := make([]ClusterSummary, len(names))
	var wg sync.WaitGroup

	for i, name := range names {
		wg.Add(1)
		go func(idx int, n string) {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			cc, err := m.GetClient(cctx, n)
			if err != nil {
				results[idx] = ClusterSummary{
					ID: n, Name: n, Health: HealthOffline,
				}
				return
			}

			summary := ClusterSummary{
				ID:          n,
				Name:        n,
				Health:      cc.Health,
				Fingerprint: cc.Fingerprint,
			}

			if v, err := cc.clientset.Discovery().ServerVersion(); err == nil {
				summary.Version = v.GitVersion
			}

			if nodes, err := cc.clientset.CoreV1().Nodes().List(cctx, metav1.ListOptions{}); err == nil {
				summary.Nodes = len(nodes.Items)
			}

			if pods, err := cc.clientset.CoreV1().Pods(metav1.NamespaceAll).List(cctx, metav1.ListOptions{}); err == nil {
				summary.PodsTotal = len(pods.Items)
				for _, p := range pods.Items {
					if p.Status.Phase == corev1.PodRunning {
						summary.PodsReady++
					}
					if p.Status.Phase != corev1.PodRunning && p.Status.Phase != corev1.PodSucceeded {
						summary.IssuesCount++
					}
				}
			}

			if nss, err := cc.clientset.CoreV1().Namespaces().List(cctx, metav1.ListOptions{}); err == nil {
				summary.Namespaces = len(nss.Items)
			}

			summary.LastUpdated = int(time.Since(cc.LastSeen).Seconds())
			results[idx] = summary
		}(i, name)
	}

	wg.Wait()
	log.Ctx(ctx).Info("clusters listed",
		zap.Int("count", len(results)),
	)
	return results
}

func (m *ClusterClientManager) healthLoop() {
	for {
		select {
		case <-m.healthTick.C:
			m.pingAll()
		case <-m.stopCh:
			return
		}
	}
}

func (m *ClusterClientManager) pingAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	m.mu.RLock()
	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		cc, err := m.GetClient(ctx, name)
		if err != nil {
			continue
		}
		cc.mu.Lock()
		_, err = cc.clientset.Discovery().ServerVersion()
		if err != nil {
			cc.Health = HealthOffline
		} else {
			cc.Health = HealthHealthy
			cc.LastSeen = time.Now()
		}
		cc.mu.Unlock()
	}
}

// Close stops the health check loop.
func (m *ClusterClientManager) Close() {
	if m.healthTick != nil {
		m.healthTick.Stop()
	}
	close(m.stopCh)
}
