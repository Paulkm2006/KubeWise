// Package config provides centralized configuration for KubeWise.
//
// Loading priority (higher wins):
//   1. Hardcoded defaults
//   2. YAML config file
//   3. Environment variables (KUBEWISE_*)
//   4. CLI flags (via ApplyFlags)
package config

import "go.uber.org/zap"

// Global holds the singleton configuration. Load() populates it;
// ApplyFlags() may further override fields from CLI flags.
var Global *Config

// L returns the global logger.  Returns zap.NewNop() when Global is nil or
// the logger has not been initialized yet — safe to call at any time.
func L() *zap.Logger {
	if Global == nil || Global.Logger == nil {
		return zap.NewNop()
	}
	return Global.Logger
}

// Config is the root configuration structure.
type Config struct {
	KubeConfig string      `json:"kubeconfig" yaml:"kubeconfig"`
	DataDir    string      `json:"data_dir"   yaml:"data_dir"`
	Verbose    bool        `json:"verbose"    yaml:"verbose"`
	Log        LogConfig   `json:"log"        yaml:"log"`
	LLM        LLMConfig   `json:"llm"        yaml:"llm"`
	Agent      AgentConfig `json:"agent"      yaml:"agent"`
	Deploy     DeployConfig `json:"deploy"    yaml:"deploy"`
	API        APIConfig   `json:"api"        yaml:"api"`

	// Logger is the process-wide zap logger.  Set by cmd/main once during
	// startup; never written after the first goroutine is launched.
	Logger *zap.Logger `json:"-" yaml:"-"`
}

// LogConfig controls logging behaviour.
type LogConfig struct {
	Level string `json:"level" yaml:"level"` // debug / info / warn / error
	File  string `json:"file"  yaml:"file"`  // stderr, stdout, or a file path

	// Runtime-only: set by ApplyFlags when the corresponding CLI flag was
	// explicitly provided (never serialised to/from YAML).
	LevelChanged bool `json:"-" yaml:"-"`
	FileChanged  bool `json:"-" yaml:"-"`
}

// LLMConfig LLM 相关配置
type LLMConfig struct {
	Model   string `json:"model"   yaml:"model"`
	APIKey  string `json:"api_key" yaml:"api_key"`
	APIBase string `json:"api_base" yaml:"api_base"`
}

// APIConfig HTTP API server 配置
type APIConfig struct {
	Addr string `json:"addr" yaml:"addr"` // listen address, e.g. ":8080"
}

// AgentConfig Agent 相关配置
type AgentConfig struct {
	MaxSteps   int              `json:"max_steps"   yaml:"max_steps"`
	Supervisor SupervisorConfig `json:"supervisor"  yaml:"supervisor"`
}

// SupervisorConfig Supervisor 相关配置
type SupervisorConfig struct {
	Enabled            bool `json:"enabled"              yaml:"enabled"`
	RepeatThreshold    int  `json:"repeat_threshold"     yaml:"repeat_threshold"`
	PingPongThreshold  int  `json:"ping_pong_threshold"  yaml:"ping_pong_threshold"`
	SameToolThreshold  int  `json:"same_tool_threshold"  yaml:"same_tool_threshold"`
	MaxExtensions      int  `json:"max_extensions"       yaml:"max_extensions"`
	ExtensionStepGrant int  `json:"extension_step_grant"  yaml:"extension_step_grant"`
	MaxEvaluatorCalls  int  `json:"max_evaluator_calls"  yaml:"max_evaluator_calls"`
}

// DeployConfig 部署 Agent 配置
type DeployConfig struct {
	ArtifactHub ArtifactHubConfig `json:"artifact_hub" yaml:"artifact_hub"`
	Helm        HelmDeployConfig  `json:"helm"          yaml:"helm"`
}

// ArtifactHubConfig Artifact Hub 搜索配置
type ArtifactHubConfig struct {
	Enabled          bool   `json:"enabled"            yaml:"enabled"`
	Timeout          string `json:"timeout"            yaml:"timeout"`           // duration string, e.g. "5s"
	SelectionTimeout string `json:"selection_timeout"  yaml:"selection_timeout"` // duration string, e.g. "10s"
}

// HelmDeployConfig Helm 部署配置
type HelmDeployConfig struct {
	WaitTimeout string `json:"wait_timeout" yaml:"wait_timeout"` // duration string, e.g. "5m"
}