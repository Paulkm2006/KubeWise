package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"
)

// Load reads configuration from YAML file and environment variables, then
// stores the result in Global.  Calling Load multiple times is safe — it
// resets Global to defaults each time.
//
// Priority: defaults < YAML file < environment variables.
//
// If cfgFile is empty Load tries the default path ~/.kubewise.yaml silently.
func Load(cfgFile string) error {
	setDefaults()

	if cfgFile != "" {
		if err := loadFile(cfgFile); err != nil {
			return fmt.Errorf("load config file %s: %w", cfgFile, err)
		}
		fmt.Printf("使用配置文件: %s\n", cfgFile)
	} else {
		// Try default location ~/.kubewise.yaml silently.
		if home, err := os.UserHomeDir(); err == nil {
			defPath := home + "/.kubewise.yaml"
			if data, err := os.ReadFile(defPath); err == nil {
				_ = yaml.Unmarshal(data, Global)
				fmt.Printf("使用配置文件: %s\n", defPath)
			}
		}
	}

	applyEnvVars()
	return nil
}

// ApplyFlags overrides Global fields whose corresponding CLI flag was
// explicitly provided.  Must be called after Load().
func ApplyFlags(fs *pflag.FlagSet) {
	tryOverride(fs, "kubeconfig", func(v string) { Global.KubeConfig = v })
	tryOverride(fs, "model", func(v string) { Global.LLM.Model = v })
	tryOverride(fs, "api-key", func(v string) { Global.LLM.APIKey = v })
	tryOverride(fs, "api-base", func(v string) { Global.LLM.APIBase = v })
	tryOverride(fs, "log-level", func(v string) {
		Global.Log.Level = v
		Global.Log.LevelChanged = true
	})
	tryOverride(fs, "log-file", func(v string) {
		Global.Log.File = v
		Global.Log.FileChanged = true
	})
	tryOverrideBool(fs, "verbose", func(v bool) { Global.Verbose = v })
	tryOverrideInt(fs, "max-steps", func(v int) { Global.Agent.MaxSteps = v })

	// --no-supervisor toggles supervisor off.
	if f := fs.Lookup("no-supervisor"); f != nil && f.Changed {
		Global.Agent.Supervisor.Enabled = false
	}

	// --addr is bound to serveCmd; it's also handled here so that
	// serveCmd.RunE can read config.Global directly.
	tryOverride(fs, "addr", func(v string) { Global.API.Addr = v })
}

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

func setDefaults() {
	Global = &Config{
		Verbose: false,
		Log: LogConfig{
			Level: "info",
			File:  "stderr",
		},
		LLM: LLMConfig{
			Model: "glm-5.1",
		},
		Agent: AgentConfig{
			MaxSteps: 20,
			Supervisor: SupervisorConfig{
				Enabled:            true,
				RepeatThreshold:    3,
				PingPongThreshold:  3,
				SameToolThreshold:  5,
				MaxExtensions:      2,
				ExtensionStepGrant: 10,
				MaxEvaluatorCalls:  2,
			},
		},
		API: APIConfig{
			Addr: ":8080",
		},
	}
}

// ---------------------------------------------------------------------------
// YAML file
// ---------------------------------------------------------------------------

func loadFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, Global)
}

// ---------------------------------------------------------------------------
// Environment variables  (KUBEWISE_*)
// ---------------------------------------------------------------------------

// envVar converts a dotted config key to the KUBEWISE_* env var name.
//   llm.api_key  →  KUBEWISE_LLM_API_KEY
//   agent.max_steps  →  KUBEWISE_AGENT_MAX_STEPS
func envVar(key string) string {
	s := strings.ToUpper(key)
	s = strings.NewReplacer("-", "_", ".", "_").Replace(s)
	return "KUBEWISE_" + s
}

func applyEnvVars() {
	setStr := func(key string, target *string) {
		if v, ok := os.LookupEnv(envVar(key)); ok {
			*target = v
		}
	}
	setInt := func(key string, target *int) {
		if v, ok := os.LookupEnv(envVar(key)); ok {
			if n, err := strconv.Atoi(v); err == nil {
				*target = n
			}
		}
	}
	setBool := func(key string, target *bool) {
		if v, ok := os.LookupEnv(envVar(key)); ok {
			*target = v == "true" || v == "1" || v == "yes"
		}
	}

	setStr("kubeconfig", &Global.KubeConfig)
	setStr("llm.model", &Global.LLM.Model)
	setStr("llm.api_key", &Global.LLM.APIKey)
	setStr("llm.api_base", &Global.LLM.APIBase)
	setStr("log.level", &Global.Log.Level)
	setStr("log.file", &Global.Log.File)
	setBool("verbose", &Global.Verbose)
	setInt("agent.max_steps", &Global.Agent.MaxSteps)

	setBool("agent.supervisor.enabled", &Global.Agent.Supervisor.Enabled)
	setInt("agent.supervisor.repeat_threshold", &Global.Agent.Supervisor.RepeatThreshold)
	setInt("agent.supervisor.ping_pong_threshold", &Global.Agent.Supervisor.PingPongThreshold)
	setInt("agent.supervisor.same_tool_threshold", &Global.Agent.Supervisor.SameToolThreshold)
	setInt("agent.supervisor.max_extensions", &Global.Agent.Supervisor.MaxExtensions)
	setInt("agent.supervisor.extension_step_grant", &Global.Agent.Supervisor.ExtensionStepGrant)
	setInt("agent.supervisor.max_evaluator_calls", &Global.Agent.Supervisor.MaxEvaluatorCalls)

	setStr("api.addr", &Global.API.Addr)

	setBool("deploy.artifact_hub.enabled", &Global.Deploy.ArtifactHub.Enabled)
	setStr("deploy.artifact_hub.timeout", &Global.Deploy.ArtifactHub.Timeout)
	setStr("deploy.artifact_hub.selection_timeout", &Global.Deploy.ArtifactHub.SelectionTimeout)
	setStr("deploy.helm.wait_timeout", &Global.Deploy.Helm.WaitTimeout)
}

// ---------------------------------------------------------------------------
// pflag overrides
// ---------------------------------------------------------------------------

func tryOverride(fs *pflag.FlagSet, name string, apply func(string)) {
	if f := fs.Lookup(name); f != nil && f.Changed {
		apply(f.Value.String())
	}
}

func tryOverrideInt(fs *pflag.FlagSet, name string, apply func(int)) {
	if f := fs.Lookup(name); f != nil && f.Changed {
		if n, err := strconv.Atoi(f.Value.String()); err == nil {
			apply(n)
		}
	}
}

func tryOverrideBool(fs *pflag.FlagSet, name string, apply func(bool)) {
	if f := fs.Lookup(name); f != nil && f.Changed {
		if b, err := strconv.ParseBool(f.Value.String()); err == nil {
			apply(b)
		}
	}
}