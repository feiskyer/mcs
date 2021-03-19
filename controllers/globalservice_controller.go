/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	networkingv1alpha1 "github.com/feiskyer/mcs/api/v1alpha1"
	"github.com/feiskyer/mcs/azureclients"
	"github.com/feiskyer/mcs/azureclients/loadbalancerclient"
	"github.com/feiskyer/mcs/azureclients/publicipclient"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceEndpoint struct {
	Cluster        string
	Namespace      string
	Service        types.NamespacedName
	ResourceGroup  string
	LoadBalancerIP string
}

// GlobalServiceReconciler reconciles a GlobalService object
type GlobalServiceReconciler struct {
	client.Client
	azureclients.AzureConfig
	Log                   logr.Logger
	Scheme                *runtime.Scheme
	LoadBalancerClient    loadbalancerclient.Interface
	PublicIPClient        publicipclient.Interface
	KubeClusterReconciler *KubeClusterReconciler
	AzureConfigSecret     string
	AzureConfigNamespace  string

	JitterPeriod time.Duration
	WorkQueue    workqueue.RateLimitingInterface
}

// +kubebuilder:rbac:groups=networking.networking.aks.io,resources=globalservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.networking.aks.io,resources=globalservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get

func (r *GlobalServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("globalservice", req.NamespacedName)

	if r.SubscriptionID == "" {
		log.Info("Initializing Azure clients from secret")
		if err := r.InitializeAzureClient(); err != nil {
			log.Error(err, "unable to initialize Azure clients")
			return ctrl.Result{}, err
		}
	}

	var globalService networkingv1alpha1.GlobalService
	if err := r.Get(ctx, req.NamespacedName, &globalService); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("GlobalService not found")
			return ctrl.Result{}, r.reconcileGLB(&globalService, false)
		}

		log.Error(err, "unable to fetch GlobalService")
		return ctrl.Result{}, err
	}

	if !globalService.ObjectMeta.DeletionTimestamp.IsZero() {
		// Delete the global load balancer rule
		log.Info("Deleting global load balancer rule because the global service is under deleting")
		return ctrl.Result{}, r.reconcileGLB(&globalService, false)
	}

	if ret, err := r.reconcileServiceEndpoints(&globalService); err != nil {
		return ret, err
	}

	if len(globalService.Status.Endpoints) == 0 {
		// Delete the global load balancer rule
		log.Info("Deleting global load balancer rule because no endpints found for global service")
		return ctrl.Result{}, r.reconcileGLB(&globalService, false)
	}

	if err := r.reconcileGLB(&globalService, true); err != nil {
		log.Error(err, "unable to reconcile global load balancer")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	}

	return ctrl.Result{}, nil
}

func (r *GlobalServiceReconciler) reconcileServiceEndpoints(globalService *networkingv1alpha1.GlobalService) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("globalservice", globalService.Name)
	namespacedName := types.NamespacedName{Namespace: globalService.Namespace, Name: globalService.Name}
	if len(globalService.Spec.ClusterSet) == 0 {
		return ctrl.Result{}, nil
	}

	r.KubeClusterReconciler.Lock.Lock()
	defer r.KubeClusterReconciler.Lock.Unlock()

	for _, clusterName := range globalService.Spec.ClusterSet {
		clusterNamespacedName := types.NamespacedName{Namespace: globalService.Namespace, Name: clusterName}
		if clusterManager, ok := r.KubeClusterReconciler.KubeClusterManagers[clusterNamespacedName.String()]; ok {
			client := clusterManager.GetClient()

			var cluster networkingv1alpha1.KubeCluster
			if err := r.Get(ctx, clusterNamespacedName, &cluster); err != nil {
				log.WithValues("cluster", clusterNamespacedName).Error(err, "unable to fetch KubeCluster")
				continue
			}

			var service corev1.Service
			if err := client.Get(ctx, namespacedName, &service); err != nil {
				if apierrors.IsNotFound(err) {
					log.WithValues("cluster", clusterNamespacedName).Info("service not found")
					continue
				}

				// We continue to fetch next clusters if there're something wrong on one of them.
				log.WithValues("cluster", namespacedName).Error(err, "unable to fetch Service")
				continue
			}

			loadBalancerIP := ""
			if len(service.Status.LoadBalancer.Ingress) > 0 && service.ObjectMeta.DeletionTimestamp.IsZero() {
				loadBalancerIP = service.Status.LoadBalancer.Ingress[0].IP
			}
			ret, err := r.reconcileEndpoints(ServiceEndpoint{
				Cluster:        clusterNamespacedName.String(),
				Namespace:      globalService.Namespace,
				Service:        namespacedName,
				ResourceGroup:  cluster.Spec.LoadBalancerResourceGroup,
				LoadBalancerIP: loadBalancerIP,
			})
			if err != nil {
				return ret, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *GlobalServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha1.GlobalService{}).
		Complete(r)
}

func (r *GlobalServiceReconciler) Run(stop <-chan struct{}) {
	go wait.Until(r.endpointsWorker, r.JitterPeriod, stop)
}

func (r *GlobalServiceReconciler) endpointsWorker() {
	for r.processNextEndpointWorker() {
	}
}

func (r *GlobalServiceReconciler) processNextEndpointWorker() bool {
	obj, shutdown := r.WorkQueue.Get()
	if shutdown {
		return false
	}

	// We call Done here so the workqueue knows we have finished
	// processing this item. We also must remember to call Forget if we
	// do not want this work item being re-queued. For example, we do
	// not call Forget if a transient error occurs, instead the item is
	// put back on the workqueue and attempted again after a back-off
	// period.
	defer r.WorkQueue.Done(obj)

	return r.endpointsHandler(obj)
}

func (r *GlobalServiceReconciler) endpointsHandler(obj interface{}) bool {
	var req ServiceEndpoint
	var ok bool
	if req, ok = obj.(ServiceEndpoint); !ok {
		// As the item in the workqueue is actually invalid, we call
		// Forget here else we'd go into a loop of attempting to
		// process a work item that is invalid.
		r.WorkQueue.Forget(obj)
		r.Log.Error(nil, "Queue item was not a ServiceEndpoint", "type", fmt.Sprintf("%T", obj), "value", obj)
		// Return true, don't take a break
		return true
	}
	// RunInformersAndControllers the syncHandler, passing it the namespace/Name string of the
	// resource to be synced.
	if result, err := r.reconcileEndpoints(req); err != nil {
		r.WorkQueue.AddRateLimited(req)
		r.Log.Error(err, "Reconciler error", "request", req)
		return false
	} else if result.RequeueAfter > 0 {
		// The result.RequeueAfter request will be lost, if it is returned
		// along with a non-nil error. But this is intended as
		// We need to drive to stable reconcile loops before queuing due
		// to result.RequestAfter
		r.WorkQueue.Forget(obj)
		r.WorkQueue.AddAfter(req, result.RequeueAfter)
		return true
	} else if result.Requeue {
		r.WorkQueue.AddRateLimited(req)
		return true
	}

	// Finally, if no error occurs we Forget this item so it does not
	// get queued again until another change happens.
	r.WorkQueue.Forget(obj)

	// Return true, don't take a break
	return true
}

func (r *GlobalServiceReconciler) reconcileEndpoints(req ServiceEndpoint) (ctrl.Result, error) {
	ctx := context.Background()
	r.Log.Info("processing queue item")

	var globalService networkingv1alpha1.GlobalService
	if err := r.Get(ctx, req.Service, &globalService); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		r.Log.Error(err, "unable to fetch GlobalService")
		return ctrl.Result{}, err
	}

	// Filter cluster set from global service spec.
	if len(globalService.Spec.ClusterSet) > 0 {
		clusterFound := false
		for _, cluster := range globalService.Spec.ClusterSet {
			clusterFullName := fmt.Sprintf("%s/%s", req.Namespace, cluster)
			if clusterFullName == req.Cluster {
				clusterFound = true
				break
			}
		}
		if !clusterFound {
			return ctrl.Result{}, nil
		}
	}

	// Endpoints don't need any further actions when deleting global service.
	if !globalService.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	endpoints := globalService.Status.Endpoints
	needUpdateEndpoints := false
	if req.LoadBalancerIP != "" {
		// Add loadBalancerIP to global service endpoints
		serviceFound := false
		for i := range endpoints {
			if endpoints[i].Cluster == req.Cluster {
				serviceFound = true
				if req.LoadBalancerIP != endpoints[i].IP {
					endpoints[i].IP = req.LoadBalancerIP
					needUpdateEndpoints = true
					break
				}
			}
		}
		if !serviceFound {
			endpoints = append(endpoints, networkingv1alpha1.GlobalEndpoint{
				Cluster:       req.Cluster,
				ResourceGroup: req.ResourceGroup,
				IP:            req.LoadBalancerIP,
			})
			needUpdateEndpoints = true
		}
	} else {
		// Delete loadBalancerIP to global service endpoints
		for i := range endpoints {
			if endpoints[i].Cluster == req.Cluster {
				endpoints = append(endpoints[:i], endpoints[i+1:]...)
				needUpdateEndpoints = true
				break
			}
		}
	}

	if needUpdateEndpoints {
		globalService.Status.Endpoints = endpoints
		if err := r.Status().Update(ctx, &globalService); err != nil {
			r.Log.Error(err, "unable to update GlobalService status")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}
