#!/usr/bin/env bash
# ================================================================
# KubeWise 多集群故障实验 — cleanup
#
# 删除 3 个 kind 集群，清除生成的文件
#
# 使用:
#   bash cleanup.sh         删除集群并清理
#   bash cleanup.sh --keep-clusters  只删 manifest，不删集群
# ================================================================
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
DOT_KUBE_DIR="$DIR/.kube"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${CYAN}[INFO]${NC}  $*"; }
ok()   { echo -e "${GREEN}[OK]${NC}    $*"; }

if [ "${1:-}" != "--keep-clusters" ]; then
  echo ""
  echo "══════════════════════════════════════════════"
  echo "  删除 kind 集群"
  echo "══════════════════════════════════════════════"

  for name in kw-exp-a kw-exp-b kw-exp-c; do
    if kind get clusters 2>/dev/null | grep -q "^${name}$"; then
      info "删除集群 $name ..."
      kind delete cluster --name "$name"
      ok "集群 $name 已删除"
    else
      info "集群 $name 不存在，跳过"
    fi
  done
  echo ""
fi

# 清理 .kube 沙箱目录（里面都是 kind 生成的文件）
rm -rf "$DOT_KUBE_DIR"
info "kubeconfig 沙箱已清理"

echo ""
echo -e "${GREEN}✓ 清理完成${NC}"