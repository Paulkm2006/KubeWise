# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Commands

Use these make commands for common development tasks:
```bash
# Build binary for current platform
make build

# Build binaries for all platforms (Linux, Windows, macOS)
make build-all

# Install binary to /usr/local/bin
make install

# Run all tests
make test

# Run linter (golangci-lint)
make lint

# Format code with go fmt
make fmt

# Clean up compiled binaries
make clean

# Download and tidy dependencies
make deps
```

## High-Level Architecture

KubeWise is a Kubernetes intelligent operation and maintenance Agent system that integrates LLM natural language understanding with Kubernetes API capabilities.

### Core Flow
1. **CLI Entry Point** (`cmd/main.go`): Uses Cobra for command line parsing, Viper for configuration loading (supports config file, environment variables, CLI flags). Initializes K8s client and LLM client on startup.
2. **Router Agent** (`pkg/agent/router/agent.go`): First layer that classifies user query intent into one of four types:
   - `query`: Information retrieval requests
   - `operation`: Resource modification requests (in development)
   - `troubleshooting`: Issue diagnosis requests (in development)
   - `security`: Security audit requests (in development)
   Extracts key entities (namespace, resource name, resource type) from queries and routes to the appropriate specialized agent.
3. **Query Agent** (`pkg/agent/query/agent.go`): Handles all information retrieval queries. Supports up to 5 rounds of tool calling to gather necessary information:
   - Uses dynamic tool registry to load available K8s query tools
   - Automatically parses multiple tool call formats (OpenAI, GLM, Ark, etc.)
   - Returns tool call errors to the LLM for parameter correction
   - Generates natural language responses once sufficient information is gathered

### Key Components
- **K8s Client** (`pkg/k8s/client.go`): Wraps the official Kubernetes Go client, provides unified access to cluster resources.
- **LLM Client** (`pkg/llm/client.go`): Generic client compatible with all OpenAI API-compatible LLM providers (GLM, Qwen, DeepSeek, etc.).
- **Tool Registry** (`pkg/tool/registry.go`): Dynamic tool registration system:
  - Tools are registered via `RegisterGlobal()` in their package `init()` functions
  - Automatically discovered and loaded at runtime with dependency injection
  - Supports dynamic generation of LLM function definitions
- **Tool Interface** (`pkg/tools/interface.go`): All tools implement this common interface:
  - `Name()`: Returns unique snake_case identifier for the tool
  - `Description()`: Returns human-readable function description
  - `Parameters()`: Returns JSON Schema for tool parameters
  - `Execute()`: Runs the tool with provided arguments

### Tool Organization
Query tools are located in `pkg/tools/v1/query/`, each as a separate file:
- `list_persistent_volumes.go`: List all PVs in the cluster
- `get_largest_pv.go`: Get the largest PV by capacity
- `find_pods_using_pvc.go`: Find pods using a specific PVC
- `list_pods_in_namespace.go`: List pods in a namespace
- `get_pod_resource_usage.go`: Get resource configuration for a specific pod
- `list_namespaces.go`: List all namespaces
- `list_configmaps_in_namespace.go`: List ConfigMaps in a namespace
- `get_configmap_content.go`: Get content of a specific ConfigMap
- `list_custom_resources_by_gvr.go`: List custom resources by GVR
- `get_custom_resource_by_gvr_and_name.go`: Get specific custom resource by GVR and name

## Key Conventions
- **Tool Naming**: Use snake_case for tool names (e.g., `list_persistent_volumes`)
- **Configuration**: Configuration is loaded via Viper, with priority: CLI flags > environment variables (prefix `KUBEWISE_`) > config file (`~/.kubewise.yaml`)
- **Error Handling**: Tool call errors are returned to the LLM for correction instead of failing immediately, allowing for self-healing of parameter issues
- **LLM Compatibility**: The query agent supports multiple tool call response formats out of the box, with automatic parsing and normalization
