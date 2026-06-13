package transporthttp

import (
	"path/filepath"

	"go.uber.org/zap"
	"k8s.io/client-go/util/homedir"

	activityfeedapp "github.com/kubewise/kubewise/internal/activityfeed/application"
	activityfeedsqlite "github.com/kubewise/kubewise/internal/activityfeed/infrastructure/sqlite"
	activityfeedhttp "github.com/kubewise/kubewise/internal/activityfeed/interface/http"
	"github.com/kubewise/kubewise/internal/config"
	conversationapp "github.com/kubewise/kubewise/internal/conversation/application"
	"github.com/kubewise/kubewise/internal/conversation/infrastructure/filestore"
	conversationhttp "github.com/kubewise/kubewise/internal/conversation/interface/http"
	auditapp "github.com/kubewise/kubewise/internal/audit/application"
	auditsqlite "github.com/kubewise/kubewise/internal/audit/infrastructure/sqlite"
	audithttp "github.com/kubewise/kubewise/internal/audit/interface/http"
	diagapp "github.com/kubewise/kubewise/internal/diagnosis/application"
	diagsqlite "github.com/kubewise/kubewise/internal/diagnosis/infrastructure/sqlite"
	diaghttp "github.com/kubewise/kubewise/internal/diagnosis/interface/http"
	obsapp "github.com/kubewise/kubewise/internal/observability/application"
	obscluster "github.com/kubewise/kubewise/internal/observability/infrastructure/cluster"
	obshttp "github.com/kubewise/kubewise/internal/observability/interface/http"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/runtime"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/platform/persistence"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"github.com/labstack/echo/v5"
)

type Server struct {
	echo *echo.Echo
}

func NewServer() *Server {
	e := echo.New()

	cm, err := cluster.NewClusterClientManager(config.Global.KubeConfig)
	if err != nil {
		config.L().Warn("cluster manager created with warnings", zap.Error(err))
	}

	dataDir := resolveDataDir()
	var sqldb *persistence.DB
	if d, err := persistence.Open(dataDir); err != nil {
		config.L().Warn("sqlite init failed (persistence disabled)", zap.Error(err))
	} else {
		sqldb = d
	}

	agentRT, err := newAgentRuntime(cm)
	if err != nil {
		config.L().Panic("failed to init agent runtime", zap.Error(err))
	}

	convStore, err := filestore.NewFileStore(config.Global.DataDir)
	if err != nil {
		config.L().Panic("failed to init conversation store", zap.Error(err))
	}

	var activityRepo *activityfeedsqlite.Repository
	var diagnosisRepo *diagsqlite.Repository
	var auditRepo *auditsqlite.Repository
	if sqldb != nil {
		activityRepo = activityfeedsqlite.NewRepository(sqldb.DB)
		diagnosisRepo = diagsqlite.NewRepository(sqldb.DB)
		auditRepo = auditsqlite.NewRepository(sqldb.DB)
	}

	activitySvc := activityfeedapp.NewService(activityRepo)
	conversationH := &conversationhttp.Handler{
		Chat:    conversationapp.NewChatService(agentRT),
		Session: conversationapp.NewSessionService(convStore),
	}
	diagnosisH := &diaghttp.Handler{
		Service: diagapp.NewService(diagnosisRepo, agentRT, activitySvc),
	}
	auditH := &audithttp.Handler{
		Service: auditapp.NewService(auditRepo, agentRT),
	}
	observabilityH := &obshttp.Handler{
		Service: obsapp.NewService(obscluster.NewManagerReader(cm)),
	}
	activityH := &activityfeedhttp.Handler{Service: activitySvc}

	MountRoutes(e, conversationH, diagnosisH, auditH, observabilityH, activityH)
	return &Server{echo: e}
}

func (s *Server) Start(addr string) error {
	return s.echo.Start(addr)
}

func resolveDataDir() string {
	dataDir := config.Global.DataDir
	if dataDir == "" {
		if home := homedir.HomeDir(); home != "" {
			dataDir = filepath.Join(home, ".kubewise")
		}
	}
	return dataDir
}

func newAgentRuntime(cm *cluster.ClusterClientManager) (*runtime.Runtime, error) {
	sup := config.Global.Agent.Supervisor
	return runtime.New(runtime.Config{
		LLM: llm.Config{
			Model:   config.Global.LLM.Model,
			APIKey:  config.Global.LLM.APIKey,
			APIBase: config.Global.LLM.APIBase,
		},
		KubeConfig:     config.Global.KubeConfig,
		ClusterManager: cm,
		MaxSteps:       config.Global.Agent.MaxSteps,
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
}
