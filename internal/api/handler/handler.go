package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/activity"
	"github.com/kubewise/kubewise/internal/agent/supervisor"
	"github.com/kubewise/kubewise/internal/cluster"
	"github.com/kubewise/kubewise/internal/diagnosis"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"github.com/kubewise/kubewise/internal/agent/session"
	"github.com/kubewise/kubewise/internal/agent/session/store"
	"github.com/kubewise/kubewise/internal/agent/event"
)

// StreamQuerier abstracts the router agent for testability.
type StreamQuerier interface {
	HandleQuery(query string) (string, error)
	HandleQueryStream(ctx context.Context, query, queryID string, eventCh chan<- event.Event) error
}

type pendingInteraction struct {
	queryID string
	respCh  chan<- json.RawMessage
}

type Handler struct {
	querier             StreamQuerier
	sessionStore        store.Store
	k8sClient           *cluster.Client // for single-cluster status API
	clusterManager      *cluster.ClusterClientManager
	diagnosisRunner     *diagnosis.Runner
	activityService     *activity.Service
	db                  *sql.DB
	mu                  sync.RWMutex
	pendingInteractions map[string]*pendingInteraction
}

// SetK8sClient sets the Kubernetes client for cluster status queries.
func (h *Handler) SetK8sClient(c *cluster.Client) { h.k8sClient = c }

// NewHandler creates a Handler with real K8s/LLM clients.
func NewHandler() (*Handler, error) {
	sup := config.Global.Agent.Supervisor
	sess, err := session.New(session.Config{
		LLM: llm.Config{
			Model:   config.Global.LLM.Model,
			APIKey:  config.Global.LLM.APIKey,
			APIBase: config.Global.LLM.APIBase,
		},
		KubeConfig: config.Global.KubeConfig,
		MaxSteps:   config.Global.Agent.MaxSteps,
		SupervisorCfg: supervisor.Config{
			Enabled:            sup.Enabled,
			RepeatThreshold:    sup.RepeatThreshold,
			PingPongThreshold:  sup.PingPongThreshold,
			SameToolThreshold:  sup.SameToolThreshold,
			MaxExtensions:      sup.MaxExtensions,
			ExtensionStepGrant: sup.ExtensionStepGrant,
			MaxEvaluatorCalls:  sup.MaxEvaluatorCalls,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("初始化Session失败: %w", err)
	}

	routerAgent := sess.Router
	store, err := store.NewFileStore(config.Global.DataDir)
	if err != nil {
		return nil, err
	}
	return &Handler{
		querier:             routerAgent,
		k8sClient:           sess.K8s,
		sessionStore:        store,
		clusterManager:      nil,
		diagnosisRunner:     nil,
		activityService:     nil,
		db:                  nil,
		pendingInteractions: make(map[string]*pendingInteraction),
	}, nil
}

// NewHandlerWithDeps creates a Handler with custom dependencies (for testing).
func NewHandlerWithDeps(querier StreamQuerier, store store.Store) *Handler {
	return &Handler{
		querier:             querier,
		sessionStore:        store,
		pendingInteractions: make(map[string]*pendingInteraction),
	}
}

// NewHandlerWithCluster creates a Handler with multi-cluster and diagnosis support.
func NewHandlerWithCluster(
	querier StreamQuerier,
	sessionStore store.Store,
	clusterManager *cluster.ClusterClientManager,
	diagnosisRunner *diagnosis.Runner,
	activityService *activity.Service,
	db *sql.DB,
) *Handler {
	return &Handler{
		querier:             querier,
		sessionStore:        sessionStore,
		clusterManager:      clusterManager,
		diagnosisRunner:     diagnosisRunner,
		activityService:     activityService,
		db:                  db,
		pendingInteractions: make(map[string]*pendingInteraction),
	}
}

func (h *Handler) cleanupPendingInteractions(queryID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, pi := range h.pendingInteractions {
		if pi.queryID == queryID {
			delete(h.pendingInteractions, id)
		}
	}
}
