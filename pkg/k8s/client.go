package k8s

import (
	"context"
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

// Client Kubernetes客户端封装
type Client struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
}

// NewClient 创建Kubernetes客户端
func NewClient(kubeconfigPath string) (*Client, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		// 用户指定了kubeconfig路径，只使用这个路径
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("加载指定的kubeconfig失败: %w，请检查路径是否正确", err)
		}
	} else {
		// 没有指定路径，先尝试集群内配置
		config, err = rest.InClusterConfig()
		if err != nil {
			// 集群内配置失败，尝试用户目录下的默认kubeconfig
			home := homedir.HomeDir()
			if home == "" {
				return nil, fmt.Errorf("无法找到kubeconfig配置：集群内配置失败且无法获取用户主目录")
			}
			defaultKubeconfig := filepath.Join(home, ".kube", "config")
			config, err = clientcmd.BuildConfigFromFlags("", defaultKubeconfig)
			if err != nil {
				return nil, fmt.Errorf("加载kubeconfig失败：集群内配置失败且默认路径%s也加载失败: %w", defaultKubeconfig, err)
			}
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Client{
		clientset: clientset,
		config:    config,
	}, nil
}

// ListPersistentVolumes 获取所有PV
func (c *Client) ListPersistentVolumes(ctx context.Context) ([]corev1.PersistentVolume, error) {
	pvList, err := c.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvList.Items, nil
}

// ListPersistentVolumeClaims 获取指定命名空间下的PVC
func (c *Client) ListPersistentVolumeClaims(ctx context.Context, namespace string) ([]corev1.PersistentVolumeClaim, error) {
	pvcList, err := c.clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcList.Items, nil
}

// ListPods 获取指定命名空间下的Pod
func (c *Client) ListPods(ctx context.Context, namespace string) ([]corev1.Pod, error) {
	podList, err := c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

// GetPodLogs 获取Pod日志
func (c *Client) GetPodLogs(ctx context.Context, namespace, podName, containerName string) (string, error) {
	req := c.clientset.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
		TailLines: ptr(int64(100)),
	})

	logs, err := req.DoRaw(ctx)
	if err != nil {
		return "", err
	}
	return string(logs), nil
}

// ListNamespaces 获取所有命名空间
func (c *Client) ListNamespaces(ctx context.Context) ([]corev1.Namespace, error) {
	nsList, err := c.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return nsList.Items, nil
}

// GetPod 获取指定Pod
func (c *Client) GetPod(ctx context.Context, namespace, podName string) (*corev1.Pod, error) {
	pod, err := c.clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pod, nil
}

func ptr[T any](v T) *T {
	return &v
}
