package api

import (
    "database/sql"
    "path/filepath"

    "go.uber.org/zap"
    "k8s.io/client-go/util/homedir"

    "github.com/kubewise/kubewise/internal/activity"
    "github.com/kubewise/kubewise/internal/agent/session"
    "github.com/kubewise/kubewise/internal/agent/session/store"
    "github.com/kubewise/kubewise/internal/agent/supervisor"
    "github.com/kubewise/kubewise/internal/api/handler"
    "github.com/kubewise/kubewise/internal/api/router"
    "github.com/kubewise/kubewise/internal/cluster"
    "github.com/kubewise/kubewise/internal/config"
    "github.com/kubewise/kubewise/internal/db"
    "github.com/kubewise/kubewise/internal/diagnosis"
    "github.com/kubewise/kubewise/internal/utils/llm"
    "github.com/labstack/echo/v5"
)

type Server struct {
    echo    *echo.Echo
    handler *handler.Handler
}

func NewServer() *Server {
    e := echo.New()

    // --- Cluster manager ---
    cm, err := cluster.NewClusterClientManager(config.Global.KubeConfig)
    if err != nil {
        config.L().Warn("cluster manager created with warnings", zap.Error(err))
    }

    // --- SQLite ---
    var sqldb *sql.DB
    dataDir := config.Global.DataDir
    if dataDir == "" {
        if home := homedir.HomeDir(); home != "" {
            dataDir = filepath.Join(home, ".kubewise")
        }
    }
    if d, err := db.Open(dataDir); err != nil {
        config.L().Warn("sqlite init failed (diagnosis history disabled)", zap.Error(err))
    } else {
        sqldb = d.DB
    }

    // --- Domain services ---
    diagRunner := diagnosis.NewRunner()
    var activitySvc *activity.Service
    if sqldb != nil {
        activitySvc = activity.NewService(sqldb)
    }

    // --- Session + Router Agent ---
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
        config.L().Panic("failed to init session", zap.Error(err))
    }

    sessionStore, err := store.NewFileStore(config.Global.DataDir)
    if err != nil {
        config.L().Panic("failed to init session store", zap.Error(err))
    }

    h := handler.NewHandlerWithCluster(
        sess.Router,
        sessionStore,
        cm,
        diagRunner,
        activitySvc,
        sqldb,
    )

    router.InitRouter(e, h)
    return &Server{echo: e, handler: h}
}

func (s *Server) Start(addr string) error {
    return s.echo.Start(addr)
}
