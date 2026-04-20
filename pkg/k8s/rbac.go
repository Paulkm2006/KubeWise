package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListRoles 获取指定命名空间下的Role
func (c *Client) ListRoles(ctx context.Context, namespace string) ([]rbacv1.Role, error) {
	roleList, err := c.clientset.RbacV1().Roles(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return roleList.Items, nil
}

// ListClusterRoles 获取所有ClusterRole
func (c *Client) ListClusterRoles(ctx context.Context) ([]rbacv1.ClusterRole, error) {
	crList, err := c.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return crList.Items, nil
}

// ListRoleBindings 获取指定命名空间下的RoleBinding
func (c *Client) ListRoleBindings(ctx context.Context, namespace string) ([]rbacv1.RoleBinding, error) {
	rbList, err := c.clientset.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return rbList.Items, nil
}

// ListClusterRoleBindings 获取所有ClusterRoleBinding
func (c *Client) ListClusterRoleBindings(ctx context.Context) ([]rbacv1.ClusterRoleBinding, error) {
	crbList, err := c.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return crbList.Items, nil
}

// ListServiceAccounts 获取指定命名空间下的ServiceAccount
func (c *Client) ListServiceAccounts(ctx context.Context, namespace string) ([]corev1.ServiceAccount, error) {
	saList, err := c.clientset.CoreV1().ServiceAccounts(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return saList.Items, nil
}
