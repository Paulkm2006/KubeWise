package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

// ScaleResource sets the replica count for a Deployment or StatefulSet.
func (c *Client) ScaleResource(ctx context.Context, namespace, kind, name string, replicas int32) error {
	switch strings.ToLower(kind) {
	case "deployment":
		scale, err := c.clientset.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		scale.Spec.Replicas = replicas
		_, err = c.clientset.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		return err
	case "statefulset":
		scale, err := c.clientset.AppsV1().StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		scale.Spec.Replicas = replicas
		_, err = c.clientset.AppsV1().StatefulSets(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("ScaleResource: unsupported kind %s", kind)
	}
}

// RestartResource triggers a rolling restart by patching the restartedAt annotation.
func (c *Client) RestartResource(ctx context.Context, namespace, kind, name string) error {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						"kubectl.kubernetes.io/restartedAt": time.Now().UTC().Format(time.RFC3339),
					},
				},
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	switch strings.ToLower(kind) {
	case "deployment":
		_, err = c.clientset.AppsV1().Deployments(namespace).Patch(ctx, name, k8stypes.MergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	case "statefulset":
		_, err = c.clientset.AppsV1().StatefulSets(namespace).Patch(ctx, name, k8stypes.MergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	case "daemonset":
		_, err = c.clientset.AppsV1().DaemonSets(namespace).Patch(ctx, name, k8stypes.MergePatchType, patchBytes, metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("RestartResource: unsupported kind %s", kind)
	}
}

// DeleteResource deletes any resource via the dynamic client.
func (c *Client) DeleteResource(ctx context.Context, namespace string, gvr schema.GroupVersionResource, name string) error {
	if namespace != "" {
		return c.dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return c.dynamicClient.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
}

// ApplyResource parses YAML and applies it via Server-Side Apply.
func (c *Client) ApplyResource(ctx context.Context, yamlContent string) error {
	jsonBytes, err := yaml.YAMLToJSON([]byte(yamlContent))
	if err != nil {
		return fmt.Errorf("ApplyResource: failed to convert YAML to JSON: %w", err)
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal(jsonBytes, &obj.Object); err != nil {
		return fmt.Errorf("ApplyResource: failed to unmarshal object: %w", err)
	}
	gvr := gvrFromUnstructured(&obj)
	forceTrue := true
	ns := obj.GetNamespace()
	opts := metav1.PatchOptions{FieldManager: "kubewise", Force: &forceTrue}
	if ns == "" {
		_, err = c.dynamicClient.Resource(gvr).Patch(ctx, obj.GetName(), k8stypes.ApplyPatchType, jsonBytes, opts)
	} else {
		_, err = c.dynamicClient.Resource(gvr).Namespace(ns).Patch(ctx, obj.GetName(), k8stypes.ApplyPatchType, jsonBytes, opts)
	}
	return err
}

// gvrFromUnstructured derives a GroupVersionResource from an Unstructured object.
func gvrFromUnstructured(obj *unstructured.Unstructured) schema.GroupVersionResource {
	gvk := obj.GroupVersionKind()
	resource := strings.ToLower(gvk.Kind) + "s"
	return schema.GroupVersionResource{
		Group:   gvk.Group,
		Version: gvk.Version,
		Resource: resource,
	}
}

// CordonNode sets or unsets spec.unschedulable on a node.
func (c *Client) CordonNode(ctx context.Context, nodeName string, cordon bool) error {
	patch := map[string]any{"spec": map[string]any{"unschedulable": cordon}}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = c.clientset.CoreV1().Nodes().Patch(ctx, nodeName, k8stypes.MergePatchType, patchBytes, metav1.PatchOptions{})
	return err
}

// DrainNode evicts all non-DaemonSet, non-mirror Pods from a node.
// Returns lists of evicted and remaining (failed) pod identifiers.
func (c *Client) DrainNode(ctx context.Context, nodeName string) (evicted []string, remaining []string, err error) {
	podList, err := c.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("DrainNode: failed to list pods on node %s: %w", nodeName, err)
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if isDaemonSetPod(pod) || isMirrorPod(pod) {
			continue
		}
		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: pod.Namespace},
		}
		evictErr := c.clientset.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction)
		if evictErr != nil {
			remaining = append(remaining, pod.Namespace+"/"+pod.Name)
		} else {
			evicted = append(evicted, pod.Namespace+"/"+pod.Name)
		}
	}
	return evicted, remaining, nil
}

// isDaemonSetPod returns true if the pod is owned by a DaemonSet.
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// isMirrorPod returns true if the pod is a static/mirror pod.
func isMirrorPod(pod *corev1.Pod) bool {
	_, ok := pod.Annotations["kubernetes.io/config.mirror"]
	return ok
}

// LabelResource applies labels and/or annotations to any resource via merge patch.
func (c *Client) LabelResource(ctx context.Context, namespace string, gvr schema.GroupVersionResource, name string, labels, annotations map[string]string) error {
	patch := map[string]any{"metadata": map[string]any{}}
	meta := patch["metadata"].(map[string]any)
	if len(labels) > 0 {
		meta["labels"] = labels
	}
	if len(annotations) > 0 {
		meta["annotations"] = annotations
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	if namespace != "" {
		_, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Patch(ctx, name, k8stypes.MergePatchType, patchBytes, metav1.PatchOptions{})
	} else {
		_, err = c.dynamicClient.Resource(gvr).Patch(ctx, name, k8stypes.MergePatchType, patchBytes, metav1.PatchOptions{})
	}
	return err
}
