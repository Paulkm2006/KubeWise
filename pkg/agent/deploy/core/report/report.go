// Package report builds user-facing deployment success summaries.
package report

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
)

// SuccessMessage builds the final Helm success report shown to the user.
func SuccessMessage(ctx context.Context, rel *helm.Release, chartInfo *catalog.ChartInfo, namespace, releaseName string, k8sClient *k8s.Client, log *zap.Logger) string {
	if log == nil {
		log = zap.NewNop()
	}
	if rel == nil {
		return fmt.Sprintf("✅ %s 部署完成", chartInfo.ChartName)
	}
	ns := rel.Namespace
	if ns == "" {
		ns = namespace
	}
	rn := rel.Name
	if rn == "" {
		rn = releaseName
	}
	verifyNote := workloadNote(ctx, k8sClient, ns, rn, log)
	return fmt.Sprintf(`✅ Helm 部署成功

Release:   %s
Namespace: %s
Chart:     %s (%s)
Status:    %s
%s
提示：kubectl get all -n %s`,
		rn,
		ns,
		chartInfo.ChartName,
		rel.Chart,
		rel.Status,
		verifyNote,
		ns,
	)
}

func workloadNote(ctx context.Context, k8sClient *k8s.Client, namespace, releaseName string, log *zap.Logger) string {
	if k8sClient == nil || namespace == "" {
		return ""
	}
	pods, err := k8sClient.ListPods(ctx, namespace)
	if err != nil {
		log.Warn("post-deploy pod check failed", zap.String("namespace", namespace), zap.Error(err))
		return fmt.Sprintf("\n⚠️ 无法检查命名空间 %s 中的 Pod: %v\n", namespace, err)
	}
	if len(pods) == 0 {
		log.Warn("helm deployed but no pods in namespace",
			zap.String("namespace", namespace),
			zap.String("release", releaseName),
		)
		return fmt.Sprintf(`
⚠️ 命名空间 %s 内没有 Pod。Helm 状态为 deployed 不代表主应用已运行。
常见原因：选错了 Chart（例如 argocd-apps 不会安装 Argo CD 本体），或 values 为空未启用任何组件。
请检查 Chart 选择，或执行 helm get manifest %s -n %s 查看实际创建的资源。
`, namespace, releaseName, namespace)
	}
	running := 0
	for _, p := range pods {
		switch p.Status.Phase {
		case "Running", "Pending":
			running++
		}
	}
	log.Info("post-deploy pod check",
		zap.String("namespace", namespace),
		zap.Int("pods", len(pods)),
		zap.Int("active", running),
	)
	if running == 0 {
		return fmt.Sprintf("\n⚠️ 命名空间 %s 有 %d 个 Pod，但无 Running/Pending 状态，请 kubectl describe pod -n %s\n", namespace, len(pods), namespace)
	}
	return fmt.Sprintf("\nPod: %d 个（%d Running/Pending）\n", len(pods), running)
}
