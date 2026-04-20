.PHONY: all build clean test install

# 项目信息
BINARY_NAME = kubewise
VERSION = 0.1.0
BUILD_TIME = $(shell date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT = $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")

# 编译参数
LDFLAGS = -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)"

all: build

# 编译二进制
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd

# 编译Linux版本
build-linux:
	@echo "Building Linux amd64 version..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 ./cmd

# 编译Windows版本
build-windows:
	@echo "Building Windows amd64 version..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe ./cmd

# 编译macOS版本
build-darwin:
	@echo "Building macOS amd64 version..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 ./cmd

# 编译所有平台
build-all: build-linux build-windows build-darwin

# 安装到系统路径
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	sudo mv $(BINARY_NAME) /usr/local/bin/

# 运行测试
test:
	@echo "Running tests..."
	go test -v ./pkg/...

# 清理编译产物
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME) $(BINARY_NAME)-*
	rm -f *.out

# 代码格式化
fmt:
	@echo "Formatting code..."
	go fmt ./...

# 代码检查
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# 下载依赖
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy
