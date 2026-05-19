package config

import "time"

// Config 全局配置
type Config struct {
	KubeConfig string       `mapstructure:"kubeconfig"`
	LLM        LLMConfig    `mapstructure:"llm"`
	Deploy     DeployConfig `mapstructure:"deploy"`
	Log        LogConfig    `mapstructure:"log"`
}

// LLMConfig LLM相关配置
type LLMConfig struct {
	Model   string `mapstructure:"model"`
	APIKey  string `mapstructure:"api_key"`
	APIBase string `mapstructure:"api_base"`
}

// LogConfig 日志相关配置（需配合 -v 命令行 flag 启用）
type LogConfig struct {
	// Level 日志级别：debug / info / warn / error（-v 默认 debug）
	Level string `mapstructure:"level"`
	// File 日志文件路径（-v 默认 ./kubewise.log）
	File string `mapstructure:"file"`
}

// DeployConfig 部署 Agent 配置
type DeployConfig struct {
	ArtifactHub ArtifactHubConfig `mapstructure:"artifact_hub"`
	Helm        HelmDeployConfig  `mapstructure:"helm"`
}

// ArtifactHubConfig Artifact Hub 搜索配置
type ArtifactHubConfig struct {
	Enabled          bool          `mapstructure:"enabled"`
	Timeout          time.Duration `mapstructure:"timeout"`
	SelectionTimeout time.Duration `mapstructure:"selection_timeout"`
}

// HelmDeployConfig Helm 部署配置
type HelmDeployConfig struct {
	WaitTimeout time.Duration `mapstructure:"wait_timeout"`
}
