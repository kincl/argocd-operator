package argocd

import (
	"errors"
	"testing"

	"gotest.tools/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/rbac/v1"

	argoprojv1alpha1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
)

var errMsg = errors.New("this is a test error")

func testDeploymentHook(cr *argoprojv1alpha1.ArgoCD, v interface{}, s string) error {
	switch o := v.(type) {
	case *appsv1.Deployment:
		var replicas int32 = 3
		o.Spec.Replicas = &replicas
	}
	return nil
}

func testClusterRoleHook(cr *argoprojv1alpha1.ArgoCD, v interface{}, s string) error {
	switch o := v.(type) {
	case *v1.ClusterRole:
		o.Rules = append(o.Rules, policyRuleForApplicationController()...)
	}
	return nil
}

func testRoleHook(cr *argoprojv1alpha1.ArgoCD, v interface{}, s string) error {
	switch o := v.(type) {
	case *v1.Role:
		if o.Name == cr.Name+"-"+applicationController {
			o.Rules = append(o.Rules, testRules()...)
		}
	}
	return nil
}

func testErrorHook(cr *argoprojv1alpha1.ArgoCD, v interface{}, s string) error {
	return errMsg
}

func TestReconcileArgoCD_testDeploymentHook(t *testing.T) {
	defer resetHooks()()
	a := makeTestArgoCD()

	Register(testDeploymentHook)

	testDeployment := makeTestDeployment()

	assert.NilError(t, applyReconcilerHook(a, testDeployment, ""))
	var expectedReplicas int32 = 3
	assert.DeepEqual(t, &expectedReplicas, testDeployment.Spec.Replicas)
}

func TestReconcileArgoCD_testMultipleHooks(t *testing.T) {
	defer resetHooks()()
	a := makeTestArgoCD()

	testDeployment := makeTestDeployment()
	testClusterRole := makeTestClusterRole()

	Register(testDeploymentHook)
	Register(testClusterRoleHook)

	assert.NilError(t, applyReconcilerHook(a, testDeployment, ""))
	assert.NilError(t, applyReconcilerHook(a, testClusterRole, ""))

	// Verify if testDeploymentHook is executed successfully
	var expectedReplicas int32 = 3
	assert.DeepEqual(t, &expectedReplicas, testDeployment.Spec.Replicas)

	// Verify if testClusterRoleHook is executed successfully
	want := append(makeTestPolicyRules(), policyRuleForApplicationController()...)
	assert.DeepEqual(t, want, testClusterRole.Rules)
}

func TestReconcileArgoCD_hooks_end_upon_error(t *testing.T) {
	defer resetHooks()()
	a := makeTestArgoCD()
	Register(testErrorHook, testClusterRoleHook)

	testClusterRole := makeTestClusterRole()

	assert.Error(t, applyReconcilerHook(a, testClusterRole, ""), "this is a test error")
	assert.DeepEqual(t, makeTestPolicyRules(), testClusterRole.Rules)
}

func resetHooks() func() {
	origDefaultHooksFunc := hooks

	return func() {
		hooks = origDefaultHooksFunc
	}
}
