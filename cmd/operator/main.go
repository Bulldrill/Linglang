// cmd/operator runs the LinLang Kubernetes operator (issue #21): watches
// HilbertSpace custom resources and reconciles a Deployment+Service of
// quantum-node (#20) pods for each one.
//
// Run out-of-cluster against your current kubeconfig context:
//
//	go run ./cmd/operator/ -image linlang-quantum-node:latest
package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	linlangv1alpha1 "linlang-go/api/v1alpha1"
	"linlang-go/internal/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(linlangv1alpha1.AddToScheme(scheme))
}

func main() {
	image := flag.String("image", "linlang-quantum-node:latest", "imagen de quantum-node a desplegar")
	metricsAddr := flag.String("metrics-bind-address", ":8080", "dirección para el endpoint de métricas")
	probeAddr := flag.String("health-probe-bind-address", ":8081", "dirección para liveness/readiness")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: *metricsAddr},
		HealthProbeBindAddress: *probeAddr,
	})
	if err != nil {
		ctrl.Log.Error(err, "no se pudo iniciar el manager")
		os.Exit(1)
	}

	if err := (&controller.HilbertSpaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Image:  *image,
	}).SetupWithManager(mgr); err != nil {
		ctrl.Log.Error(err, "no se pudo registrar HilbertSpaceReconciler")
		os.Exit(1)
	}

	ctrl.Log.Info("arrancando el operador de LinLang", "image", *image)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Error(err, "error corriendo el manager")
		os.Exit(1)
	}
}
