package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/api"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/log"
	"github.com/kubewise/kubewise/pkg/tui"
)

var (
	cfgFile string
	logger  *zap.Logger
)

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

		// 初始化K8s客户端
		kubeconfig := viper.GetString("kubeconfig")
		k8sClient, err := k8s.NewClient(kubeconfig)
		if err != nil {
			return fmt.Errorf("初始化K8s客户端失败: %w", err)
		}
		k8sClient.SetLogger(logger)

		// 初始化LLM客户端
		llmConfig := llm.Config{
			Model:   viper.GetString("llm.model"),
			APIKey:  viper.GetString("llm.api_key"),
			APIBase: viper.GetString("llm.api_base"),
		}
		llmClient, err := llm.NewClient(llmConfig)
		if err != nil {
			return fmt.Errorf("初始化LLM客户端失败: %w", err)
		}
		llmClient.SetLogger(logger)

		// 初始化路由Agent
		routerAgent, err := router.New(k8sClient, llmClient, viper.GetInt("agent.max_steps"), getSupervisorConfig())
		if err != nil {
			return fmt.Errorf("初始化路由Agent失败: %w", err)
		}
		routerAgent.SetLogger(logger)

		// 处理查询
		fmt.Println("\n处理中...")
		result, err := routerAgent.HandleQuery(userQuery)
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
		kubeconfig := viper.GetString("kubeconfig")
		k8sClient, err := k8s.NewClient(kubeconfig)
		if err != nil {
			return fmt.Errorf("初始化K8s客户端失败: %w", err)
		}
		k8sClient.SetLogger(logger)

		llmConfig := llm.Config{
			Model:   viper.GetString("llm.model"),
			APIKey:  viper.GetString("llm.api_key"),
			APIBase: viper.GetString("llm.api_base"),
		}
		llmClient, err := llm.NewClient(llmConfig)
		if err != nil {
			return fmt.Errorf("初始化LLM客户端失败: %w", err)
		}
		llmClient.SetLogger(logger)

		return tui.Run(k8sClient, llmClient, logger, viper.GetInt("agent.max_steps"), getSupervisorConfig())
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
		kubeconfig := viper.GetString("kubeconfig")
		k8sClient, err := k8s.NewClient(kubeconfig)
		if err != nil {
			return fmt.Errorf("初始化K8s客户端失败: %w", err)
		}

		llmConfig := llm.Config{
			Model:   viper.GetString("llm.model"),
			APIKey:  viper.GetString("llm.api_key"),
			APIBase: viper.GetString("llm.api_base"),
		}
		llmClient, err := llm.NewClient(llmConfig)
		if err != nil {
			return fmt.Errorf("初始化LLM客户端失败: %w", err)
		}

		addr := viper.GetString("api.addr")
		handler, err := api.NewHandler(k8sClient, llmClient, viper.GetInt("agent.max_steps"), getSupervisorConfig())
		if err != nil {
			return fmt.Errorf("初始化API Handler失败: %w", err)
		}

		srv := api.NewServer(handler)
		logger.Info("starting API server", zap.String("addr", addr))
		return srv.Start(addr)
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig, initLogger)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(serveCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认 $HOME/.kubewise.yaml)")
	rootCmd.PersistentFlags().StringP("kubeconfig", "k", "", "kubeconfig文件路径")
	rootCmd.PersistentFlags().StringP("model", "m", "glm-5.1", "LLM模型名称")
	rootCmd.PersistentFlags().StringP("api-key", "a", "", "LLM API Key")
	rootCmd.PersistentFlags().StringP("api-base", "b", "", "LLM API Base URL")
	rootCmd.PersistentFlags().String("log-level", "info", "日志级别: debug / info / warn / error")
	rootCmd.PersistentFlags().String("log-file", "stderr", "日志文件路径 (stderr 或文件路径)")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "启用详细日志（默认 debug，写入 kubewise.log，不干扰 TUI）")
	rootCmd.PersistentFlags().Int("max-steps", 20, "Agent最大工具调用轮次")
	rootCmd.PersistentFlags().Bool("no-supervisor", false, "禁用supervisor自动干预")

	viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	viper.BindPFlag("llm.model", rootCmd.PersistentFlags().Lookup("model"))
	viper.BindPFlag("llm.api_key", rootCmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("llm.api_base", rootCmd.PersistentFlags().Lookup("api-base"))
	viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log.file", rootCmd.PersistentFlags().Lookup("log-file"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("agent.max_steps", rootCmd.PersistentFlags().Lookup("max-steps"))

	serveCmd.Flags().String("addr", ":8080", "API server listen address")
	viper.BindPFlag("api.addr", serveCmd.Flags().Lookup("addr"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取用户目录失败: %v\n", err)
			os.Exit(1)
		}
		viper.AddConfigPath(home)
		viper.SetConfigType("yaml")
		viper.SetConfigName(".kubewise")
	}

	// 配置环境变量替换，把中划线和点转成下划线
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()
	viper.SetEnvPrefix("KUBEWISE")

	viper.SetDefault("agent.max_steps", 20)
	viper.SetDefault("agent.supervisor.enabled", true)
	viper.SetDefault("agent.supervisor.repeat_threshold", 3)
	viper.SetDefault("agent.supervisor.ping_pong_threshold", 3)
	viper.SetDefault("agent.supervisor.same_tool_threshold", 5)
	viper.SetDefault("agent.supervisor.max_extensions", 2)
	viper.SetDefault("agent.supervisor.extension_step_grant", 10)
	viper.SetDefault("agent.supervisor.max_evaluator_calls", 2)

	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("使用配置文件: %s\n", viper.ConfigFileUsed())
	}
}

func initLogger() {
	levelFlag := rootCmd.PersistentFlags().Lookup("log-level")
	levelChanged := levelFlag != nil && levelFlag.Changed
	fileFlag := rootCmd.PersistentFlags().Lookup("log-file")
	fileChanged := fileFlag != nil && fileFlag.Changed

	var err error
	logger, err = log.New(log.Options{
		Verbose:          viper.GetBool("verbose"),
		Level:            viper.GetString("log.level"),
		File:             viper.GetString("log.file"),
		LevelFlagChanged: levelChanged,
		FileFlagChanged:  fileChanged,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
}

// getSupervisorConfig builds a supervisor.Config from Viper, respecting the --no-supervisor flag.
func getSupervisorConfig() supervisor.Config {
	cfg := supervisor.DefaultConfig()
	if noSup, _ := rootCmd.PersistentFlags().GetBool("no-supervisor"); noSup {
		cfg.Enabled = false
	}
	cfg.RepeatThreshold = viper.GetInt("agent.supervisor.repeat_threshold")
	cfg.PingPongThreshold = viper.GetInt("agent.supervisor.ping_pong_threshold")
	cfg.SameToolThreshold = viper.GetInt("agent.supervisor.same_tool_threshold")
	cfg.MaxExtensions = viper.GetInt("agent.supervisor.max_extensions")
	cfg.ExtensionStepGrant = viper.GetInt("agent.supervisor.extension_step_grant")
	cfg.MaxEvaluatorCalls = viper.GetInt("agent.supervisor.max_evaluator_calls")
	if viper.IsSet("agent.supervisor.enabled") {
		cfg.Enabled = viper.GetBool("agent.supervisor.enabled")
	}
	return cfg
}
