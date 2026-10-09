// Package controller implements the LinLang Kubernetes operator (issue
// #21): HilbertSpaceReconciler turns a HilbertSpace custom resource into
// a running Deployment+Service of quantum-node (#20) pods, and reports
// their readiness and entanglement state back onto the resource's status.
package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	linlangv1alpha1 "linlang-go/api/v1alpha1"
)

const (
	quantumNodePort = 50051
	labelManagedBy  = "app.kubernetes.io/managed-by"
	labelInstance   = "hilbertspace.linlang.dev/name"
)

// HilbertSpaceReconciler manages the Deployment+Service pair backing a
// HilbertSpace's quantum-node pods: CRD in, desired cluster state
// (Deployment, Service) out, continuously reconciled to match — the pod
// lifecycle management and scaling the issue asks the operator to own.
type HilbertSpaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Image is the quantum-node container image to run (Dockerfile.quantum-node).
	Image string
}

// +kubebuilder:rbac:groups=linlang.dev,resources=hilbertspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=linlang.dev,resources=hilbertspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *HilbertSpaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var hs linlangv1alpha1.HilbertSpace
	if err := r.Get(ctx, req.NamespacedName, &hs); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // deleted: owned Deployment/Service garbage-collected via OwnerReferences
		}
		return ctrl.Result{}, err
	}

	if err := r.reconcileDeployment(ctx, &hs); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling deployment: %w", err)
	}
	if err := r.reconcileService(ctx, &hs); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling service: %w", err)
	}

	var current appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: deploymentName(&hs), Namespace: hs.Namespace}, &current); err != nil {
		return ctrl.Result{}, fmt.Errorf("reading back deployment: %w", err)
	}

	hs.Status.ReadyReplicas = current.Status.ReadyReplicas
	switch {
	case hs.Spec.Replicas > 0 && current.Status.ReadyReplicas >= hs.Spec.Replicas:
		hs.Status.Phase = linlangv1alpha1.PhaseReady
	case current.Status.Replicas > 0:
		hs.Status.Phase = linlangv1alpha1.PhaseProvisioning
	default:
		hs.Status.Phase = linlangv1alpha1.PhasePending
	}
	if err := r.Status().Update(ctx, &hs); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}

	log.Info("reconciled HilbertSpace", "name", hs.Name, "phase", hs.Status.Phase,
		"ready", hs.Status.ReadyReplicas, "desired", hs.Spec.Replicas)
	return ctrl.Result{}, nil
}

func deploymentName(hs *linlangv1alpha1.HilbertSpace) string { return hs.Name + "-quantum-node" }
func serviceName(hs *linlangv1alpha1.HilbertSpace) string    { return hs.Name + "-quantum-node" }

func podLabels(hs *linlangv1alpha1.HilbertSpace) map[string]string {
	return map[string]string{
		labelManagedBy: "linlang-operator",
		labelInstance:  hs.Name,
	}
}

func (r *HilbertSpaceReconciler) reconcileDeployment(ctx context.Context, hs *linlangv1alpha1.HilbertSpace) error {
	replicas := hs.Spec.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	labels := podLabels(hs)

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deploymentName(hs), Namespace: hs.Namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "quantum-node",
						Image: r.Image,
						// IfNotPresent: a :latest-tagged image otherwise
						// defaults to Always, which tries to pull from a
						// registry even when the image was already loaded
						// onto the node directly (e.g. `kind load
						// docker-image`) — exactly how this is tested
						// locally and how a CI-built image not yet
						// published anywhere would need to run.
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args:            []string{"-addr", fmt.Sprintf(":%d", quantumNodePort)},
						Ports:           []corev1.ContainerPort{{ContainerPort: quantumNodePort, Name: "grpc"}},
						Env: []corev1.EnvVar{
							{Name: "LINLANG_QUANTUM_BACKEND", Value: hs.Spec.Technology},
						},
					}},
				},
			},
		}
		return controllerutil.SetControllerReference(hs, deploy, r.Scheme)
	})
	return err
}

func (r *HilbertSpaceReconciler) reconcileService(ctx context.Context, hs *linlangv1alpha1.HilbertSpace) error {
	labels := podLabels(hs)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: serviceName(hs), Namespace: hs.Namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Spec.Selector = labels
		svc.Spec.Ports = []corev1.ServicePort{{
			Name: "grpc", Port: quantumNodePort, TargetPort: intstr.FromInt32(quantumNodePort),
		}}
		return controllerutil.SetControllerReference(hs, svc, r.Scheme)
	})
	return err
}

// SetupWithManager registers this reconciler with mgr, watching
// HilbertSpace resources and the Deployments/Services it owns (so a
// manual kubectl edit of either gets reconciled back, not just CRD edits).
func (r *HilbertSpaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&linlangv1alpha1.HilbertSpace{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
