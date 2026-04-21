package config

// Config 全局配置
type Config struct {
	KubeConfig string    `mapstructure:"kubeconfig"`
	LLM        LLMConfig `mapstructure:"llm"`
}

// LLMConfig LLM相关配置
type LLMConfig struct {
	Model   string `mapstructure:"model"`
	APIKey  string `mapstructure:"api_key"`
	APIBase string `mapstructure:"api_base"`
}
