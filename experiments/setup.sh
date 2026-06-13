#!/usr/bin/env bash
# ================================================================
# KubeWise 多集群故障实验 — setup
#
# 一键：检测环境 → 创建 3 个 kind 集群 → 部署应用 + 故障注入
#
# 使用:
#   bash setup.sh               普通模式
#   bash setup.sh --skip-prechecks  跳过环境检测（快速重跑）
# ================================================================
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
KUBE_DIR="$DIR/kube"
DOT_KUBE_DIR="$DIR/.kube"        # kind 操作的沙箱目录，不污染 ~/.kube/config
MANIFESTS="$DIR/manifests"
CLUSTERS="$DIR/clusters"

KUBECONFIG_A="$DOT_KUBE_DIR/kind-a.kubeconfig"
KUBECONFIG_B="$DOT_KUBE_DIR/kind-b.kubeconfig"
KUBECONFIG_C="$DOT_KUBE_DIR/kind-c.kubeconfig"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

info()  { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }

# ---- 检测函数 ----
check_cmd() {
  if ! command -v "$1" &>/dev/null; then
    return 1
  fi
  return 0
}

prechecks() {
  local has_error=false

  echo ""
  echo "══════════════════════════════════════════════"
  echo "  环境检测"
  echo "══════════════════════════════════════════════"
  echo ""

  # --- docker ---
  if check_cmd docker; then
    ok "docker         $(docker --version 2>/dev/null || true)"
  else
    fail "docker 未安装"
    echo "  → 安装: https://docs.docker.com/engine/install/"
    has_error=true
  fi

  # --- kind ---
  if check_cmd kind; then
    ok "kind           $(kind --version 2>/dev/null || true)"
  else
    fail "kind 未安装"
    echo "  → 安装: go install sigs.k8s.io/kind@v0.31.0"
    echo "     或参考 https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
    has_error=true
  fi

  # --- kubectl ---
  if check_cmd kubectl; then
    ok "kubectl        $(kubectl version --client --output=json 2>/dev/null | grep -o '"gitVersion":"[^"]*"' | cut -d'"' -f4 || kubectl version --client 2>&1 | head -1)"
  else
    fail "kubectl 未安装"
    echo "  → 安装: curl -LO \"https://dl.k8s.io/release/$(curl -sL https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl\""
    has_error=true
  fi

  # --- go (optional — for building KubeWise) ---
  if check_cmd go; then
    ok "go             $(go version 2>/dev/null | grep -oP 'go\S+' || true)"
  else
    warn "go 未安装（可选，只在需要编译 KubeWise 后端时需要）"
  fi

  # --- port conflict ---
  for port in 16500 16501 16502; do
    if ss -tln 2>/dev/null | grep -q ":$port "; then
      fail "端口 $port 已被占用 — kind 集群可能已在运行"
      has_error=true
    fi
  done

  echo ""
  if $has_error; then
    echo -e "${RED}✗ 环境检测未通过，请修复上述问题后重试。${NC}"
    exit 1
  else
    echo -e "${GREEN}✓ 所有依赖已就绪，开始搭建实验环境...${NC}"
    echo ""
  fi
}

# ---- 主流程 ----

if [ "${1:-}" != "--skip-prechecks" ]; then
  prechecks
fi

# 1. 准备沙箱目录: 将 kube/ 复制成 .kube/（.kube/ 不受 git 追踪，
#    kind 的 kubeconfig 写入不会污染用户的 ~/.kube/config）
echo "══════════════════════════════════════════════"
echo "  准备 kubeconfig 沙箱"
echo "══════════════════════════════════════════════"

rm -rf "$DOT_KUBE_DIR"
mkdir -p "$DOT_KUBE_DIR"
if [ -d "$KUBE_DIR" ] && [ "$(ls -A "$KUBE_DIR" 2>/dev/null)" ]; then
  cp -r "$KUBE_DIR"/* "$DOT_KUBE_DIR/"
  info "已复制 kube/ → .kube/"
else
  info "kube/ 为空或不存在，从空目录开始"
fi

# 设置 KUBECONFIG 指向 .kube/merged.kubeconfig，后续 kind create cluster
# 就不会污染 ~/.kube/config
export KUBECONFIG="$DOT_KUBE_DIR/merged.kubeconfig"
ok "沙箱目录就绪: $DOT_KUBE_DIR"

echo ""

# 2. 创建 3 个 kind 集群
echo "══════════════════════════════════════════════"
echo "  创建 kind 集群"
echo "══════════════════════════════════════════════"

CLUSTER_NAMES=("kw-exp-a" "kw-exp-b" "kw-exp-c")
CLUSTER_CONFIGS=("$CLUSTERS/kind-a.yaml" "$CLUSTERS/kind-b.yaml" "$CLUSTERS/kind-c.yaml")

for i in "${!CLUSTER_NAMES[@]}"; do
  name="${CLUSTER_NAMES[$i]}"
  cfg="${CLUSTER_CONFIGS[$i]}"

  if kind get clusters 2>/dev/null | grep -q "^${name}$"; then
    info "集群 $name 已存在，跳过创建"
  else
    info "创建集群 $name ..."
    kind create cluster --config "$cfg"
    ok "集群 $name 创建完成"
  fi
done

echo ""

# 2. 导出 kubeconfig
echo "══════════════════════════════════════════════"
echo "  导出 kubeconfig"
echo "══════════════════════════════════════════════"

export_kubeconfig() {
  local cluster_name="$1"
  local out="$2"
  kind export kubeconfig --name "$cluster_name" --kubeconfig "$out"
  # 重命名 context 为简短的集群名
  kubectl --kubeconfig "$out" config rename-context "kind-${cluster_name}" "$cluster_name" 2>/dev/null || true
}

export_kubeconfig "kw-exp-a" "$KUBECONFIG_A"
export_kubeconfig "kw-exp-b" "$KUBECONFIG_B"
export_kubeconfig "kw-exp-c" "$KUBECONFIG_C"

# 合并成一个 kubeconfig（方便 KubeWise 读取）
KUBECONFIG_MERGED="$DOT_KUBE_DIR/merged.kubeconfig"
KUBECONFIG="" kubectl config view --raw > /dev/null 2>&1 || true
# 手动合并
KUBECONFIG="$KUBECONFIG_A:$KUBECONFIG_B:$KUBECONFIG_C" \
  kubectl config view --flatten > "$KUBECONFIG_MERGED"

ok "各集群 kubeconfig 已导出:"
ok "  cluster-a → $KUBECONFIG_A"
ok "  cluster-b → $KUBECONFIG_B"
ok "  cluster-c → $KUBECONFIG_C"
ok "  合并配置  → $KUBECONFIG_MERGED"
echo ""

# 3. 等待集群就绪
echo "══════════════════════════════════════════════"
echo "  等待集群就绪"
echo "══════════════════════════════════════════════"

wait_ready() {
  local kc="$1"
  local label="$2"
  echo -n "  等待 $label ... "
  for i in $(seq 1 30); do
    if kubectl --kubeconfig "$kc" get nodes 2>/dev/null | grep -q " Ready "; then
      echo -e "${GREEN}就绪${NC}"
      return 0
    fi
    echo -n "."
    sleep 2
  done
  echo -e "${RED}超时${NC}"
  return 1
}

wait_ready "$KUBECONFIG_A" "cluster-a"
wait_ready "$KUBECONFIG_B" "cluster-b"
wait_ready "$KUBECONFIG_C" "cluster-c"
echo ""

# 4. 部署应用 + 故障
echo "══════════════════════════════════════════════"
echo "  部署应用与故障注入"
echo "══════════════════════════════════════════════"

deploy_to() {
  local kc="$1"
  local ns="${2:-default}"
  shift 2

  kubectl --kubeconfig "$kc" create ns "$ns" 2>/dev/null || true

  for manifest in "$@"; do
    local name
    name="$(basename "$manifest" .yaml)"
    echo -n "  部署 $name → $(basename "$kc" .kubeconfig)/$ns ... "
    kubectl --kubeconfig "$kc" --namespace "$ns" apply -f "$manifest" &>/dev/null
    echo -e "${GREEN}完成${NC}"
  done
}

# cluster-a: nginx + crashloop
deploy_to "$KUBECONFIG_A" "kw-experiments" \
  "$MANIFESTS/app-nginx.yaml" \
  "$MANIFESTS/fault-crashloop.yaml"

# cluster-b: nginx + oom + imagepull
deploy_to "$KUBECONFIG_B" "kw-experiments" \
  "$MANIFESTS/app-nginx.yaml" \
  "$MANIFESTS/fault-oom.yaml" \
  "$MANIFESTS/fault-imagepull.yaml"

# cluster-c: nginx + pending
deploy_to "$KUBECONFIG_C" "kw-experiments" \
  "$MANIFESTS/app-nginx.yaml" \
  "$MANIFESTS/fault-pending.yaml"

echo ""

# 5. 等待故障显现
echo "══════════════════════════════════════════════"
echo "  等待故障状态显现（约 15s）"
echo "══════════════════════════════════════════════"

sleep 5

check_pod() {
  local kc="$1" label="$2" cluster="$3"
  local status
  status="$(kubectl --kubeconfig "$kc" -n kw-experiments get pods -l "$label" -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo 'N/A')"
  echo "  $cluster / $label → $status"
}

check_pod "$KUBECONFIG_A" "experiment=crashloop" "cluster-a"
check_pod "$KUBECONFIG_B" "experiment=oom"       "cluster-b"
check_pod "$KUBECONFIG_B" "experiment=imagepull"  "cluster-b"
check_pod "$KUBECONFIG_C" "experiment=pending"    "cluster-c"

echo ""

# 6. 输出使用说明
echo "══════════════════════════════════════════════"
echo "  实验环境就绪"
echo "══════════════════════════════════════════════"
echo ""
echo -e "${GREEN}3 个 kind 集群已创建并注入故障。${NC}"
echo ""
echo "所有 kubeconfig 文件在 .kube/ 下，未污染 ~/.kube/config:"
echo "  ls -la $DOT_KUBE_DIR/"
echo ""
echo "启动 KubeWise 查看效果:"
echo ""
echo "  1. 修改 config.yaml 的 kubeconfig 路径："
echo "       kubeconfig: \"experiments/.kube/merged.kubeconfig\""
echo ""
echo "  2. 启动后端："
echo "       go build -o kubewise ./cmd && ./kubewise serve --config config.yaml -v --log-file stderr"
echo ""
echo "  3. 启动前端："
echo "       cd frontend && npx vite --host"
echo ""
echo "清理环境:"
echo "  bash experiments/cleanup.sh"