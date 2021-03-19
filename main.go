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

package main

import (
	"flag"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	networkingv1alpha1 "github.com/feiskyer/mcs/api/v1alpha1"
	"github.com/feiskyer/mcs/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = networkingv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var azureConfigSecret string
	var azureConfigNamespace string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8081", "The address the metric endpoint binds to.")
	flag.StringVar(&azureConfigSecret, "azure-config-secret", "azure-mcs-config", "The secret name of azure config.")
	flag.StringVar(&azureConfigNamespace, "azure-config-namespace", "default", "The namespace of azure config secret.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "d5473ad2.networking.aks.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	rateLimitingWorkQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "GlobalServiceReconciler")
	kubeClusterReconciler := &controllers.KubeClusterReconciler{
		Client:              mgr.GetClient(),
		WorkQueue:           rateLimitingWorkQueue,
		KubeClusterManagers: make(map[string]*controllers.KubeClusterManager),
		Log:                 ctrl.Log.WithName("controllers").WithName("KubeCluster"),
		Scheme:              mgr.GetScheme(),
	}
	if err = kubeClusterReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KubeCluster")
		os.Exit(1)
	}
	globalServiceReconciler := &controllers.GlobalServiceReconciler{
		Client:                mgr.GetClient(),
		Log:                   ctrl.Log.WithName("controllers").WithName("GlobalService"),
		Scheme:                mgr.GetScheme(),
		JitterPeriod:          time.Second,
		AzureConfigSecret:     azureConfigSecret,
		AzureConfigNamespace:  azureConfigNamespace,
		WorkQueue:             rateLimitingWorkQueue,
		KubeClusterReconciler: kubeClusterReconciler,
	}
	if err = globalServiceReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GlobalService")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	stopChan := ctrl.SetupSignalHandler()
	globalServiceReconciler.Run(stopChan)
	if err := mgr.Start(stopChan); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
