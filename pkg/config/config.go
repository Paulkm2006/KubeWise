package config

import (
	"go.uber.org/zap"
)

// Config 全局配置
type Config struct {
	KubeConfig string `mapstructure:"kubeconfig"`
	LLM        LLMConfig `mapstructure:"llm"`
}

// LLMConfig LLM相关配置
type LLMConfig struct {
	Model   string `mapstructure:"model"`
	APIKey  string `mapstructure:"api_key"`
	APIBase string `mapstructure:"api_base"`
}

var (
	globalConfig *Config
	logger       *zap.Logger
)

// InitGlobalConfig 初始化全局配置
func InitGlobalConfig(l *zap.Logger) {
	logger = l
	globalConfig = &Config{}

	// 从viper加载配置
	// 这里简化实现，后续完善
}

// GetConfig 获取全局配置
func GetConfig() *Config {
	return globalConfig
}

// GetLogger 获取日志实例
func GetLogger() *zap.Logger {
	return logger
}
