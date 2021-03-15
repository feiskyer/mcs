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

	networkingv1alpha1 "github.com/feiskyer/mcs/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type KubeClusterManager struct {
	manager.Manager

	StopChan chan struct{}
}

func (mgr *KubeClusterManager) Run() error {
	go mgr.Start(mgr.StopChan)
	return nil
}

// KubeClusterReconciler reconciles a KubeCluster object
type KubeClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	GlobalServiceReconciler *GlobalServiceReconciler
	KubeClusterManagers     map[string]*KubeClusterManager
}

// +kubebuilder:rbac:groups=networking.networking.aks.io,resources=kubeclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.networking.aks.io,resources=kubeclusters/status,verbs=get;update;patch

func (r *KubeClusterReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("kubecluster", req.NamespacedName)

	var cluster networkingv1alpha1.KubeCluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		log.Error(err, "unable to fetch KubeCluster")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Stop KubeClusterManager on cluster deletion.
	clusterName := req.NamespacedName.String()
	if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if mgr, ok := r.KubeClusterManagers[clusterName]; ok {
			close(mgr.StopChan)
			delete(r.KubeClusterManagers, clusterName)
		}
		return ctrl.Result{}, nil
	}

	// Return if KubeClusterManager has already started.
	if _, ok := r.KubeClusterManagers[clusterName]; ok {
		return ctrl.Result{}, nil
	}

	// Create new KubeClusterManager for new cluster.
	mgr, err := r.newKubeClusterManager(cluster)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := mgr.Run(); err != nil {
		return ctrl.Result{}, err
	}

	r.KubeClusterManagers[clusterName] = mgr
	return ctrl.Result{}, nil
}

func (r *KubeClusterReconciler) newKubeClusterManager(cluster networkingv1alpha1.KubeCluster) (*KubeClusterManager, error) {
	kubeconfig, err := r.getKubeConfig(cluster.Spec.KubeConfigSecret, cluster.Namespace)
	if err != nil {
		return nil, err
	}

	config, err := clientcmd.BuildConfigFromKubeconfigGetter("", func() (*clientcmdapi.Config, error) {
		return clientcmd.Load([]byte(kubeconfig))
	})
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = networkingv1alpha1.AddToScheme(scheme)
	clusterManager, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	if err != nil {
		return nil, err
	}

	namespacedName := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}
	if err = (&KubeServiceWatcher{
		Name:                    namespacedName.String(),
		Client:                  clusterManager.GetClient(),
		GlobalServiceReconciler: r.GlobalServiceReconciler,
		Log:                     ctrl.Log.WithName(namespacedName.String()),
		Scheme:                  clusterManager.GetScheme(),
		ResourceGroup:           cluster.Spec.LoadBalancerResourceGroup,
	}).SetupWithManager(clusterManager); err != nil {
		return nil, err
	}

	return &KubeClusterManager{
		StopChan: make(chan struct{}),
		Manager:  clusterManager,
	}, nil
}

func (r *KubeClusterReconciler) getKubeConfig(secretName, secretNamespace string) (string, error) {
	var secret corev1.Secret
	namespacedName := types.NamespacedName{Namespace: secretNamespace, Name: secretName}
	if err := r.Get(context.Background(), namespacedName, &secret); err != nil {
		return "", err
	}

	kubeconfig, ok := secret.Data["kubeconfig"]
	if !ok || len(kubeconfig) == 0 {
		return "", fmt.Errorf("kubeconfig not found in secret %s", namespacedName)
	}

	return string(kubeconfig), nil
}

func (r *KubeClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkingv1alpha1.KubeCluster{}).
		Complete(r)
}
