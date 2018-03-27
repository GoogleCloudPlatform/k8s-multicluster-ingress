// Copyright 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package healthcheck

import (
	"net/http"
	"testing"

	compute "google.golang.org/api/compute/v1"
	api_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	clientfake "k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/ingress-gce/pkg/annotations"
	ingressbe "k8s.io/ingress-gce/pkg/backends"
	ingresshc "k8s.io/ingress-gce/pkg/healthchecks"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

func TestEnsureHealthCheck(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	svcPort := int32(8080)
	probePath := "/test-path"
	// Should create the health check as expected.
	hcp := ingresshc.NewFakeHealthCheckProvider()
	namer := utilsnamer.NewNamer("mci1", lbName)
	hcName := namer.HealthCheckName(port)
	hcs := NewHealthCheckSyncer(namer, hcp)

	defaultClient := getClient()

	testCases := []struct {
		// Human-readable description of test.
		desc string
		// Inputs
		protocol    annotations.AppProtocol
		forceUpdate bool
		clients     map[string]kubernetes.Interface
		// Outputs
		expectedHCPath string
		ensureErr      bool
	}{
		{
			desc:           "expected no error in ensuring health check",
			protocol:       annotations.ProtocolHTTP,
			forceUpdate:    false,
			clients:        map[string]kubernetes.Interface{"client1": defaultClient, "client2": defaultClient},
			expectedHCPath: "/",
			ensureErr:      false,
		},
		{
			desc:           "writing same health check should not error (forceUpdate=false)",
			protocol:       annotations.ProtocolHTTP,
			forceUpdate:    false,
			clients:        map[string]kubernetes.Interface{"client1": defaultClient, "client2": defaultClient},
			expectedHCPath: "/",
			ensureErr:      false,
		},
		{
			desc:           "writing same health check should not error (forceUpdate=true)",
			protocol:       annotations.ProtocolHTTP,
			forceUpdate:    true,
			clients:        map[string]kubernetes.Interface{"client1": defaultClient, "client2": defaultClient},
			expectedHCPath: "/",
			ensureErr:      false,
		},
		{
			desc:           "a different healthcheck should cause an error when forceUpdate=false",
			protocol:       annotations.ProtocolHTTPS, /* Not the original HTTP */
			forceUpdate:    false,
			clients:        map[string]kubernetes.Interface{"client1": defaultClient, "client2": defaultClient},
			expectedHCPath: "/",
			ensureErr:      true,
		},
		{
			desc:           "a different healthcheck should not error when forceUpdate=true",
			protocol:       annotations.ProtocolHTTPS, /* Not the original HTTP */
			forceUpdate:    true,
			clients:        map[string]kubernetes.Interface{"client1": defaultClient, "client2": defaultClient},
			expectedHCPath: "/",
			ensureErr:      false,
		},
		{
			desc:        "a healthcheck should adopt request path of readiness probe if present",
			protocol:    annotations.ProtocolHTTP,
			forceUpdate: true,
			clients: map[string]kubernetes.Interface{
				"client1": addProbe(getClient(), annotations.ProtocolHTTP, svcPort, probePath),
				"client2": addProbe(getClient(), annotations.ProtocolHTTP, svcPort, probePath),
			},
			expectedHCPath: probePath,
			ensureErr:      false,
		},
		{
			desc:        "different readiness probe paths should cause failure",
			protocol:    annotations.ProtocolHTTP,
			forceUpdate: true,
			clients: map[string]kubernetes.Interface{
				"client1": addProbe(getClient(), annotations.ProtocolHTTP, svcPort, probePath),
				"client2": addProbe(getClient(), annotations.ProtocolHTTP, svcPort, probePath+"-a"),
			},
			expectedHCPath: probePath,
			ensureErr:      true,
		},
	}

	// GET should return NotFound.
	if _, err := hcp.GetHealthCheck(hcName); !utils.IsHTTPErrorCode(err, http.StatusNotFound) {
		t.Fatalf("expected NotFound error before EnsureHealthCheck, actual: %v", err)
	}

	for _, c := range testCases {
		hcMap, err := hcs.EnsureHealthCheck(lbName, []ingressbe.ServicePort{
			{
				NodePort: port,
				Protocol: c.protocol,
				SvcPort:  intstr.FromInt(int(svcPort)),
				SvcName: types.NamespacedName{
					Namespace: "testns",
					Name:      "testsvc",
				},
			},
		}, c.clients, c.forceUpdate)
		if (err != nil) != c.ensureErr {
			t.Errorf("error when: %v: EnsureHealthCheck({%v,%v}, %v) = [%v]. Want err? %t.",
				c.desc, port, c.protocol, c.forceUpdate, err, c.ensureErr)
		}
		if c.ensureErr == false {
			if len(hcMap) != 1 || hcMap[port] == nil {
				t.Fatalf("error when %s: unexpected health check map: %v. Expected it to contain only the health check for port %d", c.desc, hcMap, port)
			}

			hc := hcMap[port]
			if (c.protocol == annotations.ProtocolHTTP && hc.HttpHealthCheck.RequestPath != c.expectedHCPath) ||
				(c.protocol == annotations.ProtocolHTTPS && hc.HttpsHealthCheck.RequestPath != c.expectedHCPath) {
				t.Fatalf("error when %s: resulting health check did not contain the expected request path %s", c.desc, c.expectedHCPath)
			}
		}

		// Verify that GET does not return NotFound.
		if _, err := hcp.GetHealthCheck(hcName); err != nil {
			t.Fatalf("error when %s: expected nil error, actual: %v", c.desc, err)
		}

	}

	// TODO(G-Harmon): Validate values in health check.
}

func getClient() *clientfake.Clientset {
	client := clientfake.Clientset{}
	client.AddReactor("get", "services", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		ret = &api_v1.Service{}
		return true, ret, nil
	})
	return &client
}

func addProbe(client *clientfake.Clientset, protocol annotations.AppProtocol, svcPort int32, probePath string) *clientfake.Clientset {
	var scheme api_v1.URIScheme
	switch protocol {
	case annotations.ProtocolHTTP:
		scheme = api_v1.URISchemeHTTP
	case annotations.ProtocolHTTPS:
		scheme = api_v1.URISchemeHTTPS
	}
	client.AddReactor("list", "pods", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
		ret = &api_v1.PodList{
			Items: []api_v1.Pod{
				{
					Spec: api_v1.PodSpec{
						Containers: []api_v1.Container{
							{
								Ports: []api_v1.ContainerPort{
									{
										ContainerPort: svcPort,
									},
								},
								ReadinessProbe: &api_v1.Probe{
									Handler: api_v1.Handler{
										HTTPGet: &api_v1.HTTPGetAction{
											Scheme: scheme,
											Path:   probePath,
											Port:   intstr.FromInt(int(svcPort)),
										},
									},
								},
							},
						},
					},
				},
			},
		}
		return true, ret, nil
	})
	return client
}

func TestHealthCheckMatches(t *testing.T) {
	var check compute.HealthCheck
	if !healthCheckMatches(check, check) {
		t.Errorf("Want healthCheckMatches(c, c) = true. got false.")
	}
	check2 := check
	check2.Description = "foo"
	if healthCheckMatches(check, check2) {
		t.Errorf("Want healthCheckMatches(c, c2) = false, c.description differs. got true.")
	}
	check.Description = "foo"
	if !healthCheckMatches(check, check2) {
		t.Errorf("Health checks should be identical again c:%v, c2:%v", check, check2)
	}
	// CreationTimestamp should be ignored.
	check2.CreationTimestamp = "1234"
	if !healthCheckMatches(check, check2) {
		t.Errorf("Health checks only differ in creation timestamp, watch Matches(c, c2)=true, got false")
	}
}

func TestDeleteHealthCheck(t *testing.T) {
	lbName := "lb-name"
	port := int64(32211)
	// Should create the health check as expected.
	hcp := ingresshc.NewFakeHealthCheckProvider()
	namer := utilsnamer.NewNamer("mci1", lbName)
	hcName := namer.HealthCheckName(port)
	hcs := NewHealthCheckSyncer(namer, hcp)
	ports := []ingressbe.ServicePort{
		{
			NodePort: port,
			Protocol: annotations.ProtocolHTTP,
		},
	}
	if _, err := hcs.EnsureHealthCheck(lbName, ports, map[string]kubernetes.Interface{"client1": &clientfake.Clientset{}}, false); err != nil {
		t.Fatalf("unexpected error in ensuring health checks: %s", err)
	}
	if _, err := hcp.GetHealthCheck(hcName); err != nil {
		t.Fatalf("unexpected error %s, expected nil", err)
	}
	// Verify that GET fails after DELETE.
	if err := hcs.DeleteHealthChecks(ports); err != nil {
		t.Fatalf("unexpected error in deleting health checks: %s", err)
	}
	if _, err := hcp.GetHealthCheck(hcName); err == nil {
		t.Fatalf("unexpected nil error, expected NotFound")
	}
}
