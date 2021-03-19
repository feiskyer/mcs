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

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeServiceWatcher reconciles a KubeServiceWatcher object
type KubeServiceWatcher struct {
	client.Client
	Name          string
	Log           logr.Logger
	Scheme        *runtime.Scheme
	ResourceGroup string

	WorkQueue workqueue.RateLimitingInterface
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get

func (r *KubeServiceWatcher) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("KubeServiceWatcher", req.NamespacedName)

	var service corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		log.Error(err, "unable to fetch Service")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !service.ObjectMeta.DeletionTimestamp.IsZero() {
		serviceEndpoint := ServiceEndpoint{
			Cluster:        r.Name,
			Namespace:      req.Namespace,
			Service:        req.NamespacedName,
			ResourceGroup:  r.ResourceGroup,
			LoadBalancerIP: "",
		}
		r.WorkQueue.Add(serviceEndpoint)
		return ctrl.Result{}, nil
	}

	if len(service.Status.LoadBalancer.Ingress) > 0 {
		loadBalancerIP := service.Status.LoadBalancer.Ingress[0].IP
		serviceEndpoint := ServiceEndpoint{
			Cluster:        r.Name,
			Namespace:      req.Namespace,
			ResourceGroup:  r.ResourceGroup,
			Service:        req.NamespacedName,
			LoadBalancerIP: loadBalancerIP,
		}
		r.WorkQueue.Add(serviceEndpoint)
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

func (r *KubeServiceWatcher) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Service{}).
		Named(r.Name).
		Complete(r)
}
