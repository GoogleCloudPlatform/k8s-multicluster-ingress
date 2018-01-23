package ingress

import (
	"reflect"
	"sort"
	"testing"

	core_v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
)

type reactorAction struct {
	verb   string
	action func(action core.Action) (handled bool, ret runtime.Object, err error)
}

func TestEnsureIngress(t *testing.T) {

	var originalIngress, updatedIngress v1beta1.Ingress
	if err := UnmarshallAndApplyDefaults("../../../../testdata/ingress.yaml", "", &originalIngress); err != nil {
		t.Fatalf("%s", err)
	}

	updatedIngress = *originalIngress.DeepCopy()
	updatedIngress.Spec.Rules[0].HTTP.Paths[0].Path = "/bar"

	testCases := []struct {
		desc            string
		action          string
		ingress         v1beta1.Ingress
		forceUpdate     bool
		shouldFail      bool
		reactors        []reactorAction
		expectedActions []string
	}{
		{
			desc:    "expected no error in ensuring ingress",
			action:  "ensure",
			ingress: originalIngress,
			reactors: []reactorAction{
				{
					verb: "get",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						return true, &v1beta1.Ingress{}, errors.NewNotFound(schema.ParseGroupResource("extensions.ingress"), originalIngress.Name)
					},
				},
				{
					verb: "create",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						return true, action.(core.CreateAction).GetObject(), nil
					},
				},
			},
			expectedActions: []string{
				"get",
				"create",
				"get",
				"create",
			},
		},
		{
			desc:    "expected ensuring existing ingress to do nothing",
			action:  "ensure",
			ingress: originalIngress,
			reactors: []reactorAction{
				{
					verb: "get",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						oldIng := *originalIngress.DeepCopy()
						// Update an insignificant attribute to make sure we don't compare irrelevant stuff
						oldIng.Status.LoadBalancer.Ingress = append(oldIng.Status.LoadBalancer.Ingress, core_v1.LoadBalancerIngress{"127.0.0.1", "localhost"})
						return true, &oldIng, nil
					},
				},
			},
			expectedActions: []string{
				"get",
				"get",
			},
		},
		{
			desc:        "expected updating ingress without force to fail",
			action:      "ensure",
			ingress:     updatedIngress,
			forceUpdate: false,
			shouldFail:  true,
			reactors: []reactorAction{
				{
					verb: "get",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						return true, &originalIngress, nil
					},
				},
			},
		},
		{
			desc:        "expected updating ingress with force to succeed",
			action:      "ensure",
			ingress:     updatedIngress,
			forceUpdate: true,
			reactors: []reactorAction{
				{
					verb: "get",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						return true, &originalIngress, nil
					},
				},
				{
					verb: "create",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						return true, action.(core.CreateAction).GetObject(), nil
					},
				},
			},
			expectedActions: []string{
				"get",
				"update",
				"get",
				"update",
			},
		},
		{
			desc:    "expected deleting ingress to succeed",
			action:  "delete",
			ingress: originalIngress,
			reactors: []reactorAction{
				{
					verb: "delete",
					action: func(action core.Action) (handled bool, ret runtime.Object, err error) {
						return true, nil, nil
					},
				},
			},
			expectedActions: []string{
				"delete",
				"delete",
			},
		},
	}

	for _, c := range testCases {
		fakeClient := fake.Clientset{}
		for _, a := range c.reactors {
			fakeClient.AddReactor(a.verb, "ingresses", a.action)
		}
		clients := map[string]kubeclient.Interface{
			"cluster1": &fakeClient,
			"cluster2": &fakeClient,
		}

		var err error
		if c.action == "ensure" {
			var clusters []string
			clusters, err = NewIngressSyncer().EnsureIngress(&c.ingress, clients, c.forceUpdate)
			expectedClusters := []string{"cluster1", "cluster2"}
			sort.Strings(clusters)
			if !reflect.DeepEqual(clusters, expectedClusters) {
				t.Errorf("%s: unexpected list of clusters, expected: %v, got: %v", c.desc, expectedClusters, clusters)
			}
		} else if c.action == "delete" {
			err = NewIngressSyncer().DeleteIngress(&c.ingress, clients)
		}

		if c.shouldFail {
			if err == nil {
				t.Fatalf("%s: Actions attempted: %+v", c.desc, fakeClient.Actions())
			}
		} else {
			if err != nil {
				t.Fatalf("%s: %s", c.desc, err)
			}

			actions := fakeClient.Actions()
			if len(actions) != len(c.expectedActions) {
				t.Errorf("%s: Expected %d actions, got: %d\n%+v", c.desc, len(c.expectedActions), len(actions), actions)
			}
			for i, a := range actions {
				if !a.Matches(c.expectedActions[i], "ingresses") {
					t.Errorf("%s: Expected ingress %s.", c.desc, c.expectedActions[i])
				}
			}
		}
	}

	// TODO(G-Harmon): Verify that the ingress matches testdata/ingress.yaml
	// TODO(nikhiljindal): Also add tests for error cases including one for already exists.
	// TODO(nikhiljindal): Also add tests for error cases including one for does not exist.
}
