package ingress

import (
	"reflect"
	"sort"
	"testing"

	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

func TestEnsureIngress(t *testing.T) {
	var ing v1beta1.Ingress
	if err := UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "", &ing); err != nil {
		t.Fatalf("%s", err)
	}

	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("get", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, &v1beta1.Ingress{}, errors.NewNotFound(schema.ParseGroupResource("extensions.ingress"), ing.Name)
	})
	fakeClient.AddReactor("create", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, action.(core.CreateAction).GetObject(), nil
	})
	clients := map[string]kubeclient.Interface{
		"cluster1": &fakeClient,
		"cluster2": &fakeClient,
	}

	clusters, err := NewIngressSyncer().EnsureIngress(&ing, clients, false)
	if err != nil {
		t.Fatalf("%s", err)
	}
	actions := fakeClient.Actions()
	if len(actions) != 4 {
		t.Errorf("Expected 4 actions: Get Ingress 1, Create Ingress 1, Get Ingress 2, Create Ingress 2. Got:\n%+v\n%v\n%v\n%v\n%v\n%v\n", actions, len(actions), actions[0], actions[1], actions[2], actions[3])
	}
	if !actions[0].Matches("get", "ingresses") {
		t.Errorf("Expected ingress get.")
	}
	if !actions[1].Matches("create", "ingresses") {
		t.Errorf("Expected ingress creation.")
	}
	if !actions[2].Matches("get", "ingresses") {
		t.Errorf("Expected ingress get.")
	}
	if !actions[3].Matches("create", "ingresses") {
		t.Errorf("Expected ingress creation.")
	}
	expectedClusters := []string{"cluster1", "cluster2"}
	sort.Strings(clusters)
	if !reflect.DeepEqual(clusters, expectedClusters) {
		t.Errorf("unexpected list of clusters, expected: %v, got: %v", expectedClusters, clusters)
	}
	// TODO(G-Harmon): Verify that the ingress matches testdata/ingress.yaml
	// TODO(nikhiljindal): Also add tests for error cases including one for already exists.
}

func TestEnsureExistingIngressDoesNothing(t *testing.T) {
	var ing v1beta1.Ingress
	if err := UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "", &ing); err != nil {
		t.Fatalf("%s", err)
	}

	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("get", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, &ing, nil
	})
	fakeClient.AddReactor("create", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, action.(core.CreateAction).GetObject(), nil
	})
	clients := map[string]kubeclient.Interface{
		"cluster1": &fakeClient,
		"cluster2": &fakeClient,
	}
	_, err := NewIngressSyncer().EnsureIngress(&ing, clients, false)
	if err != nil {
		t.Fatalf("%s", err)
	}
	actions := fakeClient.Actions()
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions: Get Ingress 1, Get Ingress 2. Got:\n%+v\n%v\n", actions, len(actions))
	}
	if !actions[0].Matches("get", "ingresses") {
		t.Errorf("Expected ingress get.")
	}
	if !actions[1].Matches("get", "ingresses") {
		t.Errorf("Expected ingress get.")
	}
}

func TestUpdateExistingIngressFailsWithoutForce(t *testing.T) {
	var oldIng, newIng v1beta1.Ingress
	if err := UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "", &oldIng); err != nil {
		t.Fatalf("%s", err)
	}
	newIng = *oldIng.DeepCopy()
	newIng.Spec.Rules[0].HTTP.Paths[0].Path = "/bar"

	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("get", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, &oldIng, nil
	})
	fakeClient.AddReactor("create", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, action.(core.CreateAction).GetObject(), nil
	})
	clients := map[string]kubeclient.Interface{
		"cluster1": &fakeClient,
		"cluster2": &fakeClient,
	}
	_, err := NewIngressSyncer().EnsureIngress(&newIng, clients, false)
	if err == nil {
		actions := fakeClient.Actions()
		t.Fatalf("EnsureIngress tried to update existing ingress even though forceUpdate was set to 'false'.\n Actions attempted: %+v\n", actions)
	}
}

func TestUpdateExistingIngressSucceedsWithForce(t *testing.T) {
	var oldIng, newIng v1beta1.Ingress
	if err := UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "", &oldIng); err != nil {
		t.Fatalf("%s", err)
	}
	newIng = *oldIng.DeepCopy()
	newIng.Spec.Rules[0].HTTP.Paths[0].Path = "/bar"

	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("get", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, &oldIng, nil
	})
	fakeClient.AddReactor("create", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, action.(core.CreateAction).GetObject(), nil
	})
	clients := map[string]kubeclient.Interface{
		"cluster1": &fakeClient,
		"cluster2": &fakeClient,
	}
	_, err := NewIngressSyncer().EnsureIngress(&newIng, clients, true)
	if err != nil {
		t.Fatalf("%s", err)
	}
	actions := fakeClient.Actions()
	if len(actions) != 4 {
		t.Errorf("Expected 2 actions: Get Ingress 1, Create Ingress 1, Get Ingress 2, Create Ingress 2. Got:\n%+v\n%v\n", actions, len(actions))
	}
	if !actions[0].Matches("get", "ingresses") {
		t.Errorf("Expected ingress get.")
	}
	if !actions[1].Matches("update", "ingresses") {
		t.Errorf("Expected ingress update.")
	}
	if !actions[2].Matches("get", "ingresses") {
		t.Errorf("Expected ingress get.")
	}
	if !actions[3].Matches("update", "ingresses") {
		t.Errorf("Expected ingress update.")
	}
}

func TestDeleteIngress(t *testing.T) {
	var ing v1beta1.Ingress
	if err := UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "", &ing); err != nil {
		t.Fatalf("%s", err)
	}

	fakeClient := fake.Clientset{}
	fakeClient.AddReactor("delete", "ingresses", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, nil
	})
	clients := map[string]kubeclient.Interface{
		"cluster1": &fakeClient,
		"cluster2": &fakeClient,
	}

	// Verify that on calling deleteIngressInClusters, fakeClient sees 2 actions to delete the ingress.
	err := NewIngressSyncer().DeleteIngress(&ing, clients)
	if err != nil {
		t.Fatalf("%s", err)
	}

	actions := fakeClient.Actions()
	if len(actions) != 2 {
		t.Errorf("Expected 2 actions: delete ingress 1, delete ingress 2. Got:%v", actions)
	}
	if !actions[0].Matches("delete", "ingresses") {
		t.Errorf("Expected ingress deletion.")
	}
	if !actions[1].Matches("delete", "ingresses") {
		t.Errorf("Expected ingress deletion.")
	}
	// TODO(nikhiljindal): Also add tests for error cases including one for does not exist.
}
