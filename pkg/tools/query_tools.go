package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/kubewise/kubewise/pkg/k8s"
)

// QueryTools 查询工具集
type QueryTools struct {
	k8sClient *k8s.Client
}

// NewQueryTools 创建查询工具集
func NewQueryTools(k8sClient *k8s.Client) *QueryTools {
	return &QueryTools{
		k8sClient: k8sClient,
	}
}

// ListPersistentVolumes 获取所有PV信息
func (t *QueryTools) ListPersistentVolumes(ctx context.Context) (string, error) {
	pvs, err := t.k8sClient.ListPersistentVolumes(ctx)
	if err != nil {
		return "", fmt.Errorf("获取PV列表失败: %w", err)
	}

	var result strings.Builder
	result.WriteString("PV列表:\n")
	result.WriteString("名称\t\t容量\t状态\t绑定PVC\t存储类\n")
	result.WriteString("----------------------------------------\n")

	for _, pv := range pvs {
		capacity := pv.Spec.Capacity.Storage().String()
		status := string(pv.Status.Phase)
		claimRef := ""
		if pv.Spec.ClaimRef != nil {
			claimRef = fmt.Sprintf("%s/%s", pv.Spec.ClaimRef.Namespace, pv.Spec.ClaimRef.Name)
		}
		storageClass := pv.Spec.StorageClassName
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n", pv.Name, capacity, status, claimRef, storageClass))
	}

	return result.String(), nil
}

// GetLargestPV 获取最大的PV信息
func (t *QueryTools) GetLargestPV(ctx context.Context) (string, error) {
	pvs, err := t.k8sClient.ListPersistentVolumes(ctx)
	if err != nil {
		return "", fmt.Errorf("获取PV列表失败: %w", err)
	}

	if len(pvs) == 0 {
		return "集群中没有PV", nil
	}

	// 按容量排序
	sort.Slice(pvs, func(i, j int) bool {
		capI := pvs[i].Spec.Capacity.Storage().Value()
		capJ := pvs[j].Spec.Capacity.Storage().Value()
		return capI > capJ
	})

	largestPV := pvs[0]
	capacity := largestPV.Spec.Capacity.Storage().String()
	status := string(largestPV.Status.Phase)
	claimRef := ""
	if largestPV.Spec.ClaimRef != nil {
		claimRef = fmt.Sprintf("%s/%s", largestPV.Spec.ClaimRef.Namespace, largestPV.Spec.ClaimRef.Name)
	}

	return fmt.Sprintf("最大的PV是: %s\n容量: %s\n状态: %s\n绑定PVC: %s",
		largestPV.Name, capacity, status, claimRef), nil
}

// FindPodsUsingPVC 查找使用指定PVC的Pod
func (t *QueryTools) FindPodsUsingPVC(ctx context.Context, pvcName, namespace string) (string, error) {
	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}

	var usingPods []corev1.Pod
	for _, pod := range pods {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == pvcName {
				usingPods = append(usingPods, pod)
				break
			}
		}
	}

	if len(usingPods) == 0 {
		return fmt.Sprintf("没有找到使用PVC %s/%s 的Pod", namespace, pvcName), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("使用PVC %s/%s 的Pod:\n", namespace, pvcName))
	for _, pod := range usingPods {
		result.WriteString(fmt.Sprintf("- %s/%s (状态: %s)\n", pod.Namespace, pod.Name, pod.Status.Phase))
	}

	return result.String(), nil
}

// ListPodsInNamespace 列出指定命名空间下的Pod
func (t *QueryTools) ListPodsInNamespace(ctx context.Context, namespace string) (string, error) {
	pods, err := t.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取Pod列表失败: %w", err)
	}

	var result strings.Builder
	if namespace == "" {
		result.WriteString("所有命名空间的Pod列表:\n")
	} else {
		result.WriteString(fmt.Sprintf("命名空间 %s 的Pod列表:\n", namespace))
	}
	result.WriteString("命名空间\t名称\t状态\tIP\t节点\n")
	result.WriteString("----------------------------------------\n")

	for _, pod := range pods {
		podIP := pod.Status.PodIP
		nodeName := pod.Spec.NodeName
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n", pod.Namespace, pod.Name, pod.Status.Phase, podIP, nodeName))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个Pod", len(pods)))
	return result.String(), nil
}

// GetPodResourceUsage 获取Pod资源使用情况（简化实现，后续可集成metrics-server）
func (t *QueryTools) GetPodResourceUsage(ctx context.Context, podName, namespace string) (string, error) {
	pod, err := t.k8sClient.GetPod(ctx, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("获取Pod信息失败: %w", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Pod %s/%s 的资源配置:\n", namespace, podName))

	for _, container := range pod.Spec.Containers {
		result.WriteString(fmt.Sprintf("\n容器: %s\n", container.Name))
		if container.Resources.Requests != nil {
			result.WriteString(fmt.Sprintf("  请求CPU: %s\n", container.Resources.Requests.Cpu().String()))
			result.WriteString(fmt.Sprintf("  请求内存: %s\n", container.Resources.Requests.Memory().String()))
		}
		if container.Resources.Limits != nil {
			result.WriteString(fmt.Sprintf("  限制CPU: %s\n", container.Resources.Limits.Cpu().String()))
			result.WriteString(fmt.Sprintf("  限制内存: %s\n", container.Resources.Limits.Memory().String()))
		}
	}

	return result.String(), nil
}

// ListNamespaces 列出所有命名空间
func (t *QueryTools) ListNamespaces(ctx context.Context) (string, error) {
	namespaces, err := t.k8sClient.ListNamespaces(ctx)
	if err != nil {
		return "", fmt.Errorf("获取命名空间列表失败: %w", err)
	}

	var result strings.Builder
	result.WriteString("命名空间列表:\n")
	result.WriteString("名称\t状态\n")
	result.WriteString("----------------\n")

	for _, ns := range namespaces {
		result.WriteString(fmt.Sprintf("%s\t%s\n", ns.Name, ns.Status.Phase))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个命名空间", len(namespaces)))
	return result.String(), nil
}
