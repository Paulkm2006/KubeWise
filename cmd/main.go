package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/kubewise/kubewise/pkg/agent/router"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
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

		// 初始化路由Agent
		routerAgent := router.New(k8sClient, llmClient)

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

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig, initLogger)
	rootCmd.AddCommand(chatCmd)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认 $HOME/.kubewise.yaml)")
	rootCmd.PersistentFlags().StringP("kubeconfig", "k", "", "kubeconfig文件路径")
	rootCmd.PersistentFlags().StringP("model", "m", "glm-5.1", "LLM模型名称")
	rootCmd.PersistentFlags().StringP("api-key", "a", "", "LLM API Key")
	rootCmd.PersistentFlags().StringP("api-base", "b", "", "LLM API Base URL")

	viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	viper.BindPFlag("llm.model", rootCmd.PersistentFlags().Lookup("model"))
	viper.BindPFlag("llm.api_key", rootCmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("llm.api_base", rootCmd.PersistentFlags().Lookup("api-base"))
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

	if err := viper.ReadInConfig(); err == nil {
		fmt.Printf("使用配置文件: %s\n", viper.ConfigFileUsed())
	}
}

func initLogger() {
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.InfoLevel)
	config.OutputPaths = []string{"stderr"}
	config.ErrorOutputPaths = []string{"stderr"}

	var err error
	logger, err = config.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
}
