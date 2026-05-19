package config

// Config 全局配置
type Config struct {
	KubeConfig string      `mapstructure:"kubeconfig"`
	LLM        LLMConfig   `mapstructure:"llm"`
	Agent      AgentConfig `mapstructure:"agent"`
}

// LLMConfig LLM相关配置
type LLMConfig struct {
	Model   string `mapstructure:"model"`
	APIKey  string `mapstructure:"api_key"`
	APIBase string `mapstructure:"api_base"`
}

// AgentConfig Agent相关配置
type AgentConfig struct {
	MaxSteps   int              `mapstructure:"max_steps"`
	Supervisor SupervisorConfig `mapstructure:"supervisor"`
}

// SupervisorConfig Supervisor相关配置
type SupervisorConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	RepeatThreshold    int  `mapstructure:"repeat_threshold"`
	PingPongThreshold  int  `mapstructure:"ping_pong_threshold"`
	SameToolThreshold  int  `mapstructure:"same_tool_threshold"`
	MaxExtensions      int  `mapstructure:"max_extensions"`
	ExtensionStepGrant int  `mapstructure:"extension_step_grant"`
	MaxEvaluatorCalls  int  `mapstructure:"max_evaluator_calls"`
}
