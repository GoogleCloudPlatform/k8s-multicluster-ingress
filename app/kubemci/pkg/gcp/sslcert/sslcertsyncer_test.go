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

	compute "google.golang.org/api/compute/v1"

	api_v1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/ingress-gce/pkg/annotations"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
	"github.com/golang/glog"
)

func TestEnsureSSLCertsForSecret(t *testing.T) {
	lbName := "lb-name"
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

	ing, client := setupIng(true /* withSecret */)
	if _, err := scs.EnsureSSLCerts(lbName, ing, client, false /*forceUpdate*/); err != nil {
		for _, c := range testCases {
			glog.Infof("\nTest case: %s", c.desc)
			ensuredCertNames, err := scs.EnsureSSLCerts(c.lBName, ing, client, c.forceUpdate)
			if (err != nil) != c.ensureErr {
				t.Errorf("Ensuring SSL cert... expected err? %v. actual: %v", c.ensureErr, err)
			}
			if c.ensureErr {
				t.Logf("Skipping validation.")
				continue
			}
			if ensuredCertNames[0] != certName {
				t.Errorf("unexpected cert name, expected: %s, actual: %s", certName, ensuredCertNames[0])
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

func TestEnsureSSLCertsForPreShared(t *testing.T) {
	lbName := "lb-name"
	certNames := []string{"testCert1", "testCert2"}
	scp := ingresslb.NewFakeLoadBalancers("" /* name */, nil /* namer */)
	namer := utilsnamer.NewNamer("mci1", lbName)
	scs := NewSSLCertSyncer(namer, scp)
	// GET should return NotFound.
	for _, certName := range certNames {
		if _, err := scp.GetSslCertificate(certName); err == nil {
			t.Fatalf("expected NotFound error, got nil")
		}
	}
	ing, client := setupIng(false /* withSecret */)

	// EnsureSSLCerts should give an error when both the secret and pre shared annotation are missing.
	_, err := scs.EnsureSSLCerts(lbName, ing, client, false /*forceUpdate*/)
	if err == nil {
		t.Errorf("unexpected nil error, expected error since both secret and pre shared cert annotation are missing")
	}
	// Adding the annotation without a cert should still return an error.
	ing.ObjectMeta.Annotations = map[string]string{
		annotations.PreSharedCertKey: strings.Join(certNames, ","),
	}
	_, err = scs.EnsureSSLCerts(lbName, ing, client, false /*forceUpdate*/)
	if err == nil {
		t.Errorf("unexpected nil error, expected error since the pre shared cert is missing")
	}

	// EnsureSSLCerts should return success if the cert exists as expected.
	for _, certName := range certNames {
		if _, err := scp.CreateSslCertificate(&compute.SslCertificate{
			Name: certName,
		}); err != nil {
			t.Errorf("unexpected error in creating ssl cert: %s", err)
		}
	}
	ensuredCertNames, err := scs.EnsureSSLCerts(lbName, ing, client, false /*forceUpdate*/)
	if err != nil {
		t.Errorf("unexpected error in ensuring ssl cert: %s", err)
	}
	if len(ensuredCertNames) != len(certNames) {
		t.Errorf("unexpected number of ensured cert names, expected: %d, actual: %d", len(certNames), len(ensuredCertNames))
	}
	for i, certName := range certNames {
		if ensuredCertNames[i] != certName {
			t.Errorf("unexpected ensured cert name, expected: %s, actual: %s", certName, ensuredCertNames[i])
		}
	}
}

func setupIng(withSecret bool) (*v1beta1.Ingress, kubeclient.Interface) {
	// Create ingress with secret for SSL cert.
	svcName := "svcName"
	svcPort := "svcPort"
	secretName := "certSecret"
	ing := &v1beta1.Ingress{
		Spec: v1beta1.IngressSpec{
			Backend: &v1beta1.IngressBackend{
				ServiceName: svcName,
				ServicePort: intstr.FromString(svcPort),
			},
		},
	}
	client := &fake.Clientset{}
	if withSecret {
		setupIngAndClientForSecret(ing, client, secretName)
	}
	return ing, client
}

func setupIngAndClientForSecret(ing *v1beta1.Ingress, client *fake.Clientset, secretName string) {
	// Setup the secret.
	ing.Spec.TLS = []v1beta1.IngressTLS{
		{
			SecretName: secretName,
		},
	}
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
}

func TestDeleteSSLCerts(t *testing.T) {
	lbName := "lb-name"
	// Should create the ssl cert as expected.
	scp := ingresslb.NewFakeLoadBalancers("" /* name */, nil /* namer */)
	namer := utilsnamer.NewNamer("mci1", lbName)
	certName := namer.SSLCertName()
	scs := NewSSLCertSyncer(namer, scp)
	// Calling DeleteSSLCerts when cert does not exist should not return an error.
	if err := scs.DeleteSSLCerts(); err != nil {
		t.Fatalf("unexpected err while deleting SSL cert that does not exist: %s", err)
	}
	ing, client := setupIng(true /* withSecret */)
	if _, err := scs.EnsureSSLCerts(lbName, ing, client, false /*forceUpdate*/); err != nil {
		t.Fatalf("expected no error in ensuring ssl cert, actual: %v", err)
	}
	if _, err := scp.GetSslCertificate(certName); err != nil {
		t.Fatalf("expected nil error, actual: %v", err)
	}
	// Verify that GET fails after DELETE
	if err := scs.DeleteSSLCerts(); err != nil {
		t.Fatalf("unexpected err while deleting SSL cert: %s", err)
	}
	if _, err := scp.GetSslCertificate(certName); err == nil {
		t.Errorf("unexpected nil error, expected NotFound")
	}
}
