package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/session"
	"github.com/kubewise/kubewise/internal/agent/supervisor"
	"github.com/kubewise/kubewise/internal/api"
	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/tui"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"github.com/kubewise/kubewise/internal/utils/log"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "kubewise",
	Short: "KubeWise - 面向Kubernetes集群的智能自动运维Agent系统",
	Long: `KubeWise是一个将大语言模型的自然语言理解与推理能力，
	与Kubernetes丰富的API生态深度融合的智能运维系统，支持：
	- 一句话操作：自然语言转Kubernetes操作
	- 智能查询：跨资源联合推理查询
	- 自动故障排查：异常检测与根因分析
	- 安全合规检测：RBAC权限审计`,
}

var chatCmd = &cobra.Command{
	Use:   "chat [query]",
	Short: "与KubeWise进行自然语言交互",
	Long: `通过自然语言与KubeWise交互，支持查询集群信息、执行操作、排查故障等。
	示例：
	  kubewise chat "列出所有命名空间"
	  kubewise chat "哪个PV占用空间最大，挂载到了哪个Pod"
	  kubewise chat "检查default命名空间下的Pod资源配置"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("请输入查询内容")
		}
		userQuery := strings.Join(args, " ")

		sess, err := session.New(session.Config{
			LLM: llm.Config{
				Model:   config.Global.LLM.Model,
				APIKey:  config.Global.LLM.APIKey,
				APIBase: config.Global.LLM.APIBase,
			},
			KubeConfig:    config.Global.KubeConfig,
			MaxSteps:      config.Global.Agent.MaxSteps,
			SupervisorCfg: getSupervisorConfig(),
		})
		if err != nil {
			return fmt.Errorf("初始化Session失败: %w", err)
		}

		// 处理查询
		fmt.Println("\n处理中...")
		result, err := sess.Router.HandleQuery(userQuery)
		if err != nil {
			return fmt.Errorf("处理查询失败: %w", err)
		}

		fmt.Println("\n结果：")
		fmt.Println(result)
		return nil
	},
}

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "启动交互式 TUI 多轮对话模式",
	Long: `启动终端交互界面（TUI），支持多轮对话、会话管理和操作确认。
	快捷键：
	  Enter     发送消息
	  Ctrl+N    新建会话
	  Ctrl+C    中断当前查询（空闲时退出）
	  Ctrl+L    清空当前会话
	  Tab       切换焦点（侧边栏 ↔ 输入框）
	  /resume   重发被中断的消息`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sess, err := session.New(session.Config{
			LLM: llm.Config{
				Model:   config.Global.LLM.Model,
				APIKey:  config.Global.LLM.APIKey,
				APIBase: config.Global.LLM.APIBase,
			},
			KubeConfig:    config.Global.KubeConfig,
			MaxSteps:      config.Global.Agent.MaxSteps,
			SupervisorCfg: getSupervisorConfig(),
		})
		if err != nil {
			return fmt.Errorf("初始化Session失败: %w", err)
		}

		return tui.Run(sess)
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 HTTP API 服务器",
	Long: `启动 HTTP API 服务器，提供 RESTful API 和 SSE 流式接口。
	示例：
	  kubewise serve
	  kubewise serve --addr :9090`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr := config.Global.API.Addr
		if f := cmd.Flags().Lookup("addr"); f != nil && f.Changed {
			addr = f.Value.String()
		}
		srv := api.NewServer()
		config.L().Info("starting API server", zap.String("addr", addr))
		return srv.Start(addr)
	},
}

func main() {
	err := rootCmd.Execute()
	config.L().Sync()
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initialize)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serveCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认 $HOME/.kubewise.yaml)")
	rootCmd.PersistentFlags().String("data-dir", "", "数据目录 (默认 ~/.kubewise)")
	rootCmd.PersistentFlags().StringP("kubeconfig", "k", "", "kubeconfig文件路径")
	rootCmd.PersistentFlags().StringP("model", "m", "glm-5.1", "LLM模型名称")
	rootCmd.PersistentFlags().StringP("api-key", "a", "", "LLM API Key")
	rootCmd.PersistentFlags().StringP("api-base", "b", "", "LLM API Base URL")
	rootCmd.PersistentFlags().String("log-level", "info", "日志级别: debug / info / warn / error")
	rootCmd.PersistentFlags().String("log-file", "stderr", "日志文件路径 (stderr 或文件路径)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "启用详细日志（默认 debug，写入 kubewise.log，不干扰 TUI）")
	rootCmd.PersistentFlags().Int("max-steps", 20, "Agent最大工具调用轮次")
	rootCmd.PersistentFlags().Bool("no-supervisor", false, "禁用supervisor自动干预")

	serveCmd.Flags().String("addr", ":3000", "API server listen address")
}

func initialize() {
	if err := config.Load(cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	config.ApplyFlags(rootCmd.PersistentFlags())
	initLogger()
}

func initLogger() {
	cfg := config.Global
	l, err := log.New(log.Options{
		Verbose:          cfg.Verbose,
		Level:            cfg.Log.Level,
		File:             cfg.Log.File,
		LevelFlagChanged: cfg.Log.LevelChanged,
		FileFlagChanged:  cfg.Log.FileChanged,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	config.Global.Logger = l
}

// getSupervisorConfig builds a supervisor.Config from config.Global.
func getSupervisorConfig() supervisor.Config {
	sup := config.Global.Agent.Supervisor
	return supervisor.Config{
		Enabled:            sup.Enabled,
		RepeatThreshold:    sup.RepeatThreshold,
		PingPongThreshold:  sup.PingPongThreshold,
		SameToolThreshold:  sup.SameToolThreshold,
		MaxExtensions:      sup.MaxExtensions,
		ExtensionStepGrant: sup.ExtensionStepGrant,
		MaxEvaluatorCalls:  sup.MaxEvaluatorCalls,
	}
}
