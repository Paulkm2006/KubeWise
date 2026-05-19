## 怎么开发？

这里给出一个快速搭建开发环境的示例

前置工具准备
- docker
- kubectl

## 搭建开发用kind集群
这里使用kind搭建了一个3节点集群

在某个目录下创建kind-kubewise.yaml（这里在仓库根目录下新建了一个.kube目录）
```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: kubewise-dev

networking:
  apiServerAddress: "127.0.0.1"
  apiServerPort: 16443

nodes:
  - role: control-plane
    extraPortMappings:
      # 类似这样添加映射到主机的NodePort
      - containerPort: 80
        hostPort: 8080
        protocol: TCP
      - containerPort: 443
        hostPort: 8443
        protocol: TCP
  - role: worker
  - role: worker
```

拉起kind集群
```bash
# 安装kind，如果command not found，请参考https://kind.sigs.k8s.io/设置PATH
go install sigs.k8s.io/kind@v0.31.0
mkdir .kube
kind create cluster --config ./.kube/kind-kubewise.yaml
```

导出kind集群上下文到目录
```bash
kind export kubeconfig \
  --name kubewise-dev \
  --kubeconfig ./.kube/config
```

平时开发时，指定KUBECONFIG变量使得kubectl等工具使用正确的上下文，如果不想进行这一步设置，也可以将新的kind集群config合并到你的~/.kube/config中
```bash
# bash/zsh
export KUBECONFIG=./.kube/config
# fish
set -gx KUBECONFIG ./.kube/config
```

另外注意修改项目配置文件config.yaml成实际使用文件位置
```yaml
kubeconfig: "./.kube/config"
```

最后，拉起kind集群后，可以运行以下命令检查集群状态，正常输出就说明搭建完毕
```bash
kubectl get node
```

## 额外工作

kind默认不装metric server,导致无法监控和查看资源状态（比如kubectl top node目前不会生效），所以需要自行装一下

```
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

如果遇到网络问题，可以自行拉取镜像之后加载到每个kind节点
```
kind load image-archive /tmp/metrics-server-v0.8.1-amd64.tar --name kubewise-dev
```