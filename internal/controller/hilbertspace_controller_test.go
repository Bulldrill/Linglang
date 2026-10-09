// hilbertspace_controller_test.go unit-tests Reconcile's logic against a
// fake client (no real API server needed — CI-safe, no binary downloads).
// The reconcile loop was additionally verified against a real kind
// cluster during development: CRD applied, operator run out-of-cluster,
// Deployment+Service created, pod reached Running, and a core.RemoteBackend
// client executed a real gate through the Service — see the commit
// message for the full trace. That level of verification doesn't fit in
// an automated test without vendoring a kind/envtest binary into CI, so
// this test covers the reconciler's behavior in isolation instead.
package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	linlangv1alpha1 "linlang-go/api/v1alpha1"
)

func newFakeReconciler(t *testing.T, objs ...client_Object) *HilbertSpaceReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := linlangv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	runtimeObjs := make([]runtime.Object, len(objs))
	for i, o := range objs {
		runtimeObjs[i] = o
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(runtimeObjs...).
		WithStatusSubresource(&linlangv1alpha1.HilbertSpace{}).
		Build()

	return &HilbertSpaceReconciler{Client: fakeClient, Scheme: scheme, Image: "linlang-quantum-node:test"}
}

// client_Object is the minimal interface client.Object satisfies, spelled
// out here only so newFakeReconciler's signature doesn't need to import
// sigs.k8s.io/controller-runtime/pkg/client just for the alias.
type client_Object interface {
	runtime.Object
	metav1.Object
}

func TestReconcileCreatesDeploymentAndService(t *testing.T) {
	hs := &linlangv1alpha1.HilbertSpace{
		ObjectMeta: metav1.ObjectMeta{Name: "qubit-alice", Namespace: "default"},
		Spec:       linlangv1alpha1.HilbertSpaceSpec{Dim: 2, Technology: "simulator", Replicas: 2},
	}
	r := newFakeReconciler(t, hs)
	ctx := context.Background()

	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "qubit-alice", Namespace: "default"}}); err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	var deploy appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: "qubit-alice-quantum-node", Namespace: "default"}, &deploy); err != nil {
		t.Fatalf("expected a Deployment to be created: %v", err)
	}
	if *deploy.Spec.Replicas != 2 {
		t.Fatalf("expected 2 replicas, got %d", *deploy.Spec.Replicas)
	}
	container := deploy.Spec.Template.Spec.Containers[0]
	if container.Image != "linlang-quantum-node:test" {
		t.Fatalf("expected image 'linlang-quantum-node:test', got %q", container.Image)
	}
	if container.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Fatalf("expected ImagePullPolicy=IfNotPresent, got %q", container.ImagePullPolicy)
	}
	foundTechEnv := false
	for _, e := range container.Env {
		if e.Name == "LINLANG_QUANTUM_BACKEND" && e.Value == "simulator" {
			foundTechEnv = true
		}
	}
	if !foundTechEnv {
		t.Fatalf("expected LINLANG_QUANTUM_BACKEND=simulator env var, got %+v", container.Env)
	}

	var svc corev1.Service
	if err := r.Get(ctx, types.NamespacedName{Name: "qubit-alice-quantum-node", Namespace: "default"}, &svc); err != nil {
		t.Fatalf("expected a Service to be created: %v", err)
	}
	if svc.Spec.Ports[0].Port != quantumNodePort {
		t.Fatalf("expected service port %d, got %d", quantumNodePort, svc.Spec.Ports[0].Port)
	}

	// Owner references (so deleting the HilbertSpace garbage-collects both).
	if len(deploy.OwnerReferences) != 1 || deploy.OwnerReferences[0].Name != "qubit-alice" {
		t.Fatalf("expected Deployment to be owned by the HilbertSpace, got %+v", deploy.OwnerReferences)
	}
}

func TestReconcilePhaseReflectsReadiness(t *testing.T) {
	hs := &linlangv1alpha1.HilbertSpace{
		ObjectMeta: metav1.ObjectMeta{Name: "qubit-bob", Namespace: "default"},
		Spec:       linlangv1alpha1.HilbertSpaceSpec{Dim: 2, Replicas: 1},
	}
	r := newFakeReconciler(t, hs)
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "qubit-bob", Namespace: "default"}}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	var afterFirst linlangv1alpha1.HilbertSpace
	if err := r.Get(ctx, req.NamespacedName, &afterFirst); err != nil {
		t.Fatal(err)
	}
	if afterFirst.Status.Phase != linlangv1alpha1.PhasePending {
		t.Fatalf("expected Phase=Pending before any pod is ready, got %q", afterFirst.Status.Phase)
	}

	// Simulate the Deployment becoming ready (a real cluster's scheduler +
	// kubelet would do this; the fake client has no controllers of its
	// own, so the test drives the status update directly — exactly the
	// signal Reconcile reads back).
	var deploy appsv1.Deployment
	if err := r.Get(ctx, types.NamespacedName{Name: "qubit-bob-quantum-node", Namespace: "default"}, &deploy); err != nil {
		t.Fatal(err)
	}
	deploy.Status.ReadyReplicas = 1
	deploy.Status.Replicas = 1
	if err := r.Status().Update(ctx, &deploy); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("second Reconcile error: %v", err)
	}
	var afterReady linlangv1alpha1.HilbertSpace
	if err := r.Get(ctx, req.NamespacedName, &afterReady); err != nil {
		t.Fatal(err)
	}
	if afterReady.Status.Phase != linlangv1alpha1.PhaseReady {
		t.Fatalf("expected Phase=Ready once ReadyReplicas>=Spec.Replicas, got %q", afterReady.Status.Phase)
	}
	if afterReady.Status.ReadyReplicas != 1 {
		t.Fatalf("expected ReadyReplicas=1, got %d", afterReady.Status.ReadyReplicas)
	}
}

func TestReconcileMissingHilbertSpaceIsANoOp(t *testing.T) {
	r := newFakeReconciler(t)
	_, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "does-not-exist", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("expected no error reconciling a deleted/nonexistent resource, got %v", err)
	}
}
