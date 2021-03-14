# Multi-Cluster Service Operator on Azure

An operator for exposing services from multiple Kubernetes clusters by leveraging [Azure cross-region load balancer](https://docs.microsoft.com/en-us/azure/load-balancer/cross-region-overview).

![](https://docs.microsoft.com/en-us/azure/load-balancer/media/cross-region-overview/cross-region-load-balancer.png)

**Status: DRAFT**

## How to deploy MCS operator

Create Azure service principal and then create the the following `cloud-config` file:

```json
{
  "cloud": "AzurePublicCloud",
  "tenantId": "<tenantId>",
  "subscriptionId": "<subscriptionId>",
  "aadClientId": "<aadClientId>",
  "aadClientSecret": "<aadClientSecret>",
  "globalLoadBalancerName": "<glbName>",
  "globalVIPLocation": "<region>",
  "globalLoadBalancerResourceGroup": "<resourceGroup>"
}
```

Then create a secret based on this config file:

```sh
kubectl create secret generic azure-mcs-config --from-file=cloud-config
```

After that, deploy the MCS operator in MCS cluster (it could be any Kubernetes cluster):

```sh
kubectx mcs-cluster
make deploy
```

## Samples

### Create KubeCluster

Assume you have two Kubernetes clusters running on Azure, one is created via aks-engine and the other is created via AKS.

```sh
# For cluster aks-engine
kubectl create secret generic aks-engine --from-file=kubeconfig
cat <<EOF | kubectl apply -f -
apiVersion: networking.networking.aks.io/v1alpha1
kind: KubeCluster
metadata:
  name: aks-engine
  namespace: default
spec:
  loadBalancerResourceGroup: <lb-resource-group>
  kubeConfigSecret: aks-engine
EOF

# For cluster aks-cluster
kubectl create secret generic aks-cluster --from-file=kubeconfig
cat <<EOF | kubectl apply -f -
apiVersion: networking.networking.aks.io/v1alpha1
kind: KubeCluster
metadata:
  name: aks-cluster
  namespace: default
spec:
  loadBalancerResourceGroup: <mc-resource-group>
  kubeConfigSecret: aks-cluster
EOF
```

### Deploy nginx service in both cluster

MCS operator assumes the service names are same as globalservice in all clusters.

```sh
kubectx aks-engine
kubectl create deployment nginx --image nginx --save-config
kubectl expose deploy nginx --port=80 --type=LoadBalancer
kubectl get service nginx

kubectx aks-cluster
kubectl create deployment nginx --image nginx --save-config
kubectl expose deploy nginx --port=80 --type=LoadBalancer
kubectl get service nginx
```

### Create GlobalService

Switch kubeconfig back to MCS cluster and then create global service:

```sh
kubectx mcs-cluster

cat <<EOF | kubectl apply -f -
apiVersion: networking.networking.aks.io/v1alpha1
kind: GlobalService
metadata:
  name: nginx
  namespace: default
spec:
  ports:
  - name: http
    port: 80
    protocol: TCP
EOF
```

After a while, verify the VIP for the global service:

```sh
$ kubectl get globalservice nginx -o yaml
apiVersion: networking.networking.aks.io/v1alpha1
kind: GlobalService
metadata:
  name: nginx
  namespace: default
  resourceVersion: "279410693"
  selfLink: /apis/networking.networking.aks.io/v1alpha1/namespaces/default/globalservices/nginx
  uid: edb771f9-328e-4785-b94d-b70e13b10ffe
spec:
  ports:
  - name: http
    port: 80
    protocol: TCP
status:
  endpoints:
  - cluster: default/aks-engine
    ip: 52.184.9.136
    resourceGroup: "<rg1>"
  - cluster: default/aks-cluster
    ip: 20.198.185.119
    resourceGroup: "<rg2>"
  vip: 23.98.101.30

# verify the vip is available
$ curl 23.98.101.30
```
