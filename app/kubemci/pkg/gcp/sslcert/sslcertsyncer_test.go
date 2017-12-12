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

package sslcert

import (
	"strings"
	"testing"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/golang/glog"
)

func TestEnsureSSLCert(t *testing.T) {
	lbName := "lb-name"
	// Should create the ssl cert as expected.
	scp := ingresslb.NewFakeLoadBalancers("" /* name */, nil /* namer */)
	namer := utilsnamer.NewNamer("mci1", lbName)
	certName := namer.SSLCertName()
	scs := NewSSLCertSyncer(namer, scp)
	// GET should return NotFound.
	if _, err := scp.GetSslCertificate(certName); err == nil {
		t.Fatalf("expected NotFound error, got nil")
	}
	testCases := []struct {
		desc string
		// In's
		lBName      string
		forceUpdate bool
		// Out's
		ensureErr bool
	}{
		{
			desc:        "initial write (force=false)",
			lBName:      "lb-name",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write same (force=false)",
			lBName:      "lb-name",
			forceUpdate: false,
			ensureErr:   false,
		},
		{
			desc:        "write different (force=false)",
			lBName:      "lb-name2",
			forceUpdate: false,
			ensureErr:   true,
		},
		{
			desc:        "write different (force=true)",
			lBName:      "lb-name3",
			forceUpdate: true,
			ensureErr:   false,
		},
	}

	ing, client := setupIng()
	if _, err := scs.EnsureSSLCert(lbName, ing, client, false /*forceUpdate*/); err != nil {
		for _, c := range testCases {
			glog.Infof("\nTest case: %s", c.desc)
			_, err := scs.EnsureSSLCert(c.lBName, ing, client, c.forceUpdate)
			if (err != nil) != c.ensureErr {
				t.Errorf("Ensuring SSL cert... expected err? %v. actual: %v", c.ensureErr, err)
			}
			if c.ensureErr {
				t.Logf("Skipping validation.")
				continue
			}
			// Verify that GET does not return NotFound.
			cert, err := scp.GetSslCertificate(certName)
			if err != nil {
				t.Errorf("expected nil error, actual: %v", err)
			}
			if !strings.Contains(cert.Description, c.lBName) {
				t.Errorf("Expected description to contain lb name (%s). got:%s", c.lBName, cert.Description)
			}
		}
	}
}

func setupIng() (*v1beta1.Ingress, kubeclient.Interface) {
	// Create ingress with secret for SSL cert.
	svcName := "svcName"
	svcPort := "svcPort"
	secretName := "certSecret"
	ing := &v1beta1.Ingress{
		Spec: v1beta1.IngressSpec{
			TLS: []v1beta1.IngressTLS{
				{
					SecretName: secretName,
				},
			},
			Backend: &v1beta1.IngressBackend{
				ServiceName: svcName,
				ServicePort: intstr.FromString(svcPort),
			},
		},
	}
	client := &fake.Clientset{}
	// Add a reaction to return secret with a fake ssl cert.
	client.AddReactor("get", "secrets", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		ret = &api_v1.Secret{
			Data: map[string][]byte{
				api_v1.TLSCertKey:       []byte("cert"),
				api_v1.TLSPrivateKeyKey: []byte("key"),
			},
		}
		return true, ret, nil
	})
	return ing, client
}

func TestDeleteSSLCert(t *testing.T) {
	lbName := "lb-name"
	// Should create the ssl cert as expected.
	scp := ingresslb.NewFakeLoadBalancers("" /* name */, nil /* namer */)
	namer := utilsnamer.NewNamer("mci1", lbName)
	certName := namer.SSLCertName()
	scs := NewSSLCertSyncer(namer, scp)
	ing, client := setupIng()
	if _, err := scs.EnsureSSLCert(lbName, ing, client, false /*forceUpdate*/); err != nil {
		t.Fatalf("expected no error in ensuring ssl cert, actual: %v", err)
	}
	if _, err := scp.GetSslCertificate(certName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// Verify that GET fails after DELETE
	if err := scs.DeleteSSLCert(); err != nil {
		t.Fatalf("unexpected err while deleting SSL cert: %s", err)
	}
	if _, err := scp.GetSslCertificate(certName); err == nil {
		t.Errorf("unexpected nil error, expected NotFound")
	}
}
