#!/usr/bin/env pwsh
# ================================================================
# KubeWise 多集群故障实验 — cleanup (PowerShell)
#
# 删除 3 个 kind 集群，清除 .kube 沙箱目录
#
# 使用:
#   ./cleanup.ps1          删除集群并清理
#   ./cleanup.ps1 -KeepClusters  只删 .kube，不删集群
# ================================================================
param([switch]$KeepClusters)

$ErrorActionPreference = "Stop"

$DIR = $PSScriptRoot
$DOT_KUBE_DIR = "$DIR\.kube"

function info  { Write-Host "[INFO]  $args" -ForegroundColor Cyan }
function ok    { Write-Host "[OK]    $args" -ForegroundColor Green }

if (-not $KeepClusters) {
    Write-Host ""
    Write-Host "$('═' * 46)" -ForegroundColor Cyan
    Write-Host "  删除 kind 集群" -ForegroundColor Cyan
    Write-Host "$('═' * 46)" -ForegroundColor Cyan

    foreach ($name in @("kw-exp-a", "kw-exp-b", "kw-exp-c")) {
        $exists = & kind get clusters 2>$null | Select-String "^${name}$" -Quiet
        if ($exists) {
            info "删除集群 $name ..."
            & kind delete cluster --name "$name"
            ok "集群 $name 已删除"
        } else {
            info "集群 $name 不存在，跳过"
        }
    }
}

# 清理 .kube 沙箱目录
if (Test-Path "$DOT_KUBE_DIR") {
    Remove-Item -Recurse -Force "$DOT_KUBE_DIR"
    info "kubeconfig 沙箱已清理"
} else {
    info ".kube 目录不存在，无需清理"
}

Write-Host ""
Write-Host "✓ 清理完成" -ForegroundColor Green