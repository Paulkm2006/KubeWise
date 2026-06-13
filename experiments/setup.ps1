#!/usr/bin/env pwsh
# ================================================================
# KubeWise 多集群故障实验 — setup (PowerShell)
#
# 一键：检测环境 → 创建 3 个 kind 集群 → 部署应用 + 故障注入
#
# 使用:
#   ./setup.ps1                 普通模式
#   ./setup.ps1 -SkipPrechecks  跳过环境检测（快速重跑）
# ================================================================
param([switch]$SkipPrechecks)

$ErrorActionPreference = "Stop"

$DIR = $PSScriptRoot
$KUBE_DIR = "$DIR\kube"
$DOT_KUBE_DIR = "$DIR\.kube"         # kind 操作的沙箱目录，不污染 ~/.kube/config
$MANIFESTS = "$DIR\manifests"
$CLUSTERS = "$DIR\clusters"

$KUBECONFIG_A = "$DOT_KUBE_DIR\kind-a.kubeconfig"
$KUBECONFIG_B = "$DOT_KUBE_DIR\kind-b.kubeconfig"
$KUBECONFIG_C = "$DOT_KUBE_DIR\kind-c.kubeconfig"

$CLUSTER_NAMES = @("kw-exp-a", "kw-exp-b", "kw-exp-c")
$CLUSTER_CONFIGS = @(
    "$CLUSTERS\kind-a.yaml",
    "$CLUSTERS\kind-b.yaml",
    "$CLUSTERS\kind-c.yaml"
)

# ---- 日志辅助 ----
function info  { Write-Host "[INFO]  $args" -ForegroundColor Cyan }
function ok    { Write-Host "[OK]    $args" -ForegroundColor Green }
function warn  { Write-Host "[WARN]  $args" -ForegroundColor Yellow }
function fail  { Write-Host "[FAIL]  $args" -ForegroundColor Red }

function banner($title) {
    Write-Host "`n$('═' * 46)" -ForegroundColor Cyan
    Write-Host "  $title" -ForegroundColor Cyan
    Write-Host "$('═' * 46)" -ForegroundColor Cyan
    Write-Host ""
}

# ---- 检测函数 ----
function Check-Cmd($name) {
    return [bool](Get-Command "$name" -ErrorAction SilentlyContinue)
}

function Prechecks {
    $hasError = $false

    banner "环境检测"

    # --- docker ---
    if (Check-Cmd docker) {
        $ver = & docker --version 2>$null
        ok "docker         $ver"
    } else {
        fail "docker 未安装"
        Write-Host "  → 安装: https://docs.docker.com/engine/install/"
        $hasError = $true
    }

    # --- kind ---
    if (Check-Cmd kind) {
        $ver = & kind --version 2>$null
        ok "kind           $ver"
    } else {
        fail "kind 未安装"
        Write-Host "  → 安装: go install sigs.k8s.io/kind@v0.31.0"
        $hasError = $true
    }

    # --- kubectl ---
    if (Check-Cmd kubectl) {
        $ver = & kubectl version --client --output=json 2>$null | Select-String -Pattern '"gitVersion":"([^"]*)"' | ForEach-Object { $_.Matches.Groups[1].Value }
        if (-not $ver) { $ver = "unknown" }
        ok "kubectl        $ver"
    } else {
        fail "kubectl 未安装"
        Write-Host "  → 安装: curl -LO https://dl.k8s.io/release/.../bin/windows/amd64/kubectl.exe"
        $hasError = $true
    }

    # --- go (optional) ---
    if (Check-Cmd go) {
        $ver = & go version 2>$null
        ok "go             $ver"
    } else {
        warn "go 未安装（可选，只在需要编译 KubeWise 后端时需要）"
    }

    # --- port conflict ---
    foreach ($port in @(16500, 16501, 16502)) {
        $inUse = netstat -an 2>$null | Select-String ":$port " -Quiet
        if ($inUse) {
            fail "端口 $port 已被占用 — kind 集群可能已在运行"
            $hasError = $true
        }
    }

    if ($hasError) {
        Write-Host "`n✗ 环境检测未通过，请修复上述问题后重试。" -ForegroundColor Red
        exit 1
    } else {
        Write-Host "`n✓ 所有依赖已就绪，开始搭建实验环境..." -ForegroundColor Green
        Write-Host ""
    }
}

# ---- 主流程 ----

if (-not $SkipPrechecks) {
    Prechecks
}

# 1. 准备沙箱目录
banner "准备 kubeconfig 沙箱"

if (Test-Path "$DOT_KUBE_DIR") {
    Remove-Item -Recurse -Force "$DOT_KUBE_DIR"
}
New-Item -ItemType Directory -Force -Path "$DOT_KUBE_DIR" | Out-Null

if ((Test-Path "$KUBE_DIR") -and (@(Get-ChildItem "$KUBE_DIR").Count -gt 0)) {
    Copy-Item -Recurse "$KUBE_DIR\*" "$DOT_KUBE_DIR\"
    info "已复制 kube/ → .kube/"
} else {
    info "kube/ 为空或不存在，从空目录开始"
}

# 设置 KUBECONFIG 指向 .kube/merged.kubeconfig，kind create cluster 不碰 ~/.kube/config
$env:KUBECONFIG = "$DOT_KUBE_DIR\merged.kubeconfig"
ok "沙箱目录就绪: $DOT_KUBE_DIR"

# 2. 创建 3 个 kind 集群
banner "创建 kind 集群"

for ($i = 0; $i -lt $CLUSTER_NAMES.Length; $i++) {
    $name = $CLUSTER_NAMES[$i]
    $cfg = $CLUSTER_CONFIGS[$i]

    $existing = & kind get clusters 2>$null | Select-String "^${name}$" -Quiet
    if ($existing) {
        info "集群 $name 已存在，跳过创建"
    } else {
        info "创建集群 $name ..."
        & kind create cluster --config "$cfg"
        if ($LASTEXITCODE -ne 0) { throw "kind create cluster $name 失败" }
        ok "集群 $name 创建完成"
    }
}

# 3. 导出 kubeconfig
banner "导出 kubeconfig"

function Export-Kubeconfig($clusterName, $outPath) {
    & kind export kubeconfig --name "$clusterName" --kubeconfig "$outPath"
    & kubectl --kubeconfig "$outPath" config rename-context "kind-${clusterName}" "$clusterName" 2>$null
}

Export-Kubeconfig "kw-exp-a" "$KUBECONFIG_A"
Export-Kubeconfig "kw-exp-b" "$KUBECONFIG_B"
Export-Kubeconfig "kw-exp-c" "$KUBECONFIG_C"

$KUBECONFIG_MERGED = "$DOT_KUBE_DIR\merged.kubeconfig"

# 合并 kubeconfig（PowerShell 用 ; 作路径分隔符）
$env:KUBECONFIG = "$KUBECONFIG_A;$KUBECONFIG_B;$KUBECONFIG_C"
& kubectl config view --flatten > "$KUBECONFIG_MERGED"

ok "各集群 kubeconfig 已导出:"
ok "  cluster-a → $KUBECONFIG_A"
ok "  cluster-b → $KUBECONFIG_B"
ok "  cluster-c → $KUBECONFIG_C"
ok "  合并配置  → $KUBECONFIG_MERGED"

# 4. 等待集群就绪
banner "等待集群就绪"

function Wait-Ready($kc, $label) {
    Write-Host "  等待 $label ... " -NoNewline
    for ($i = 0; $i -lt 30; $i++) {
        $ready = & kubectl --kubeconfig "$kc" get nodes 2>$null | Select-String " Ready " -Quiet
        if ($ready) {
            Write-Host "就绪" -ForegroundColor Green
            return $true
        }
        Write-Host "." -NoNewline
        Start-Sleep -Seconds 2
    }
    Write-Host "超时" -ForegroundColor Red
    return $false
}

Wait-Ready "$KUBECONFIG_A" "cluster-a"
Wait-Ready "$KUBECONFIG_B" "cluster-b"
Wait-Ready "$KUBECONFIG_C" "cluster-c"

# 5. 部署应用 + 故障
banner "部署应用与故障注入"

function Deploy-To($kc, $ns = "default", $manifests) {
    & kubectl --kubeconfig "$kc" create ns "$ns" 2>$null | Out-Null
    foreach ($manifest in $manifests) {
        $name = [System.IO.Path]::GetFileNameWithoutExtension($manifest)
        $clusterName = [System.IO.Path]::GetFileNameWithoutExtension($kc)
        Write-Host "  部署 $name → ${clusterName}/${ns} ... " -NoNewline
        & kubectl --kubeconfig "$kc" --namespace "$ns" apply -f "$manifest" 2>&1 | Out-Null
        Write-Host "完成" -ForegroundColor Green
    }
}

Deploy-To "$KUBECONFIG_A" "kw-experiments" @(
    "$MANIFESTS\app-nginx.yaml",
    "$MANIFESTS\fault-crashloop.yaml"
)

Deploy-To "$KUBECONFIG_B" "kw-experiments" @(
    "$MANIFESTS\app-nginx.yaml",
    "$MANIFESTS\fault-oom.yaml",
    "$MANIFESTS\fault-imagepull.yaml"
)

Deploy-To "$KUBECONFIG_C" "kw-experiments" @(
    "$MANIFESTS\app-nginx.yaml",
    "$MANIFESTS\fault-pending.yaml"
)

# 6. 等待故障显现
banner "等待故障状态显现（约 15s）"

Start-Sleep -Seconds 5

function Check-Pod($kc, $label, $cluster) {
    $status = & kubectl --kubeconfig "$kc" -n kw-experiments get pods -l "$label" -o jsonpath='{.items[0].status.phase}' 2>$null
    if (-not $status) { $status = "N/A" }
    Write-Host "  $cluster / $label → $status"
}

Check-Pod "$KUBECONFIG_A" "experiment=crashloop" "cluster-a"
Check-Pod "$KUBECONFIG_B" "experiment=oom"       "cluster-b"
Check-Pod "$KUBECONFIG_B" "experiment=imagepull"  "cluster-b"
Check-Pod "$KUBECONFIG_C" "experiment=pending"    "cluster-c"

# 7. 输出使用说明
banner "实验环境就绪"

Write-Host "3 个 kind 集群已创建并注入故障。" -ForegroundColor Green
Write-Host ""
Write-Host "所有 kubeconfig 文件在 .kube/ 下，未污染 ~/.kube/config:"
Write-Host "  ls $DOT_KUBE_DIR/"
Write-Host ""
Write-Host "启动 KubeWise 查看效果:"
Write-Host ""
Write-Host "  1. 修改 config.yaml 的 kubeconfig 路径："
Write-Host "       kubeconfig: `"experiments/.kube/merged.kubeconfig`""
Write-Host ""
Write-Host "  2. 启动后端："
Write-Host "       go build -o kubewise ./cmd && ./kubewise serve --config config.yaml -v --log-file stderr"
Write-Host ""
Write-Host "  3. 启动前端："
Write-Host "       cd frontend && npx vite --host"
Write-Host ""
Write-Host "清理环境:"
Write-Host "  bash experiments/cleanup.sh  (或 .\experiments\cleanup.ps1)"
Write-Host ""