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
	"fmt"
	"net/http"
	"reflect"

	compute "google.golang.org/api/compute/v1"

	"github.com/golang/glog"
	"k8s.io/api/extensions/v1beta1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/ingress-gce/pkg/annotations"
	ingresslb "k8s.io/ingress-gce/pkg/loadbalancers"
	"k8s.io/ingress-gce/pkg/tls"
	"k8s.io/ingress-gce/pkg/utils"

	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

// SSLCertSyncer manages GCP ssl certs for multicluster GCP L7 load balancers.
type SSLCertSyncer struct {
	namer *utilsnamer.Namer
	// Instance of SSLCertProvider interface for calling GCE SSLCert APIs.
	// There is no separate SSLCertProvider interface, so we use the bigger LoadBalancers interface here.
	scp ingresslb.LoadBalancers
}

func NewSSLCertSyncer(namer *utilsnamer.Namer, scp ingresslb.LoadBalancers) SSLCertSyncerInterface {
	return &SSLCertSyncer{
		namer: namer,
		scp:   scp,
	}
}

// Ensure this implements SSLCertSyncerInterface.
var _ SSLCertSyncerInterface = &SSLCertSyncer{}

// See interfaces.go comment.
func (s *SSLCertSyncer) EnsureSSLCert(lbName string, ing *v1beta1.Ingress, client kubeclient.Interface, forceUpdate bool) (string, error) {
	fmt.Println("Ensuring ssl cert")
	annotations := annotations.IngAnnotations(ing.ObjectMeta.Annotations)
	if annotations.UseNamedTLS() != "" {
		return s.ensurePreSharedSSLCert(lbName, ing, forceUpdate)
	}
	return s.ensureSecretSSLCert(lbName, ing, client, forceUpdate)
}

func (s *SSLCertSyncer) ensurePreSharedSSLCert(lbName string, ing *v1beta1.Ingress, forceUpdate bool) (string, error) {
	ingAnnotations := annotations.IngAnnotations(ing.ObjectMeta.Annotations)
	certName := ingAnnotations.UseNamedTLS()
	if certName == "" {
		return "", fmt.Errorf("unexpected empty value for %s annotation", annotations.PreSharedCertKey)
	}
	// Fetch the certificate and return its self link.
	cert, err := s.scp.GetSslCertificate(certName)
	if err != nil {
		return "", err
	}
	return cert.SelfLink, nil
}

func (s *SSLCertSyncer) ensureSecretSSLCert(lbName string, ing *v1beta1.Ingress, client kubeclient.Interface, forceUpdate bool) (string, error) {
	var err error
	desiredCert, err := s.desiredSSLCert(lbName, ing, client)
	if err != nil {
		err = fmt.Errorf("error %s in computing desired ssl cert", err)
		fmt.Println(err)
		return "", err
	}
	name := desiredCert.Name
	// Check if ssl cert already exists.
	existingCert, err := s.scp.GetSslCertificate(name)
	if err == nil {
		fmt.Println("ssl cert", name, "exists already. Checking if it matches our desired ssl cert", name)
		// SSL cert with that name exists already. Check if it matches what we want.
		if sslCertMatches(*desiredCert, *existingCert) {
			// Nothing to do. Desired ssl cert exists already.
			fmt.Println("Desired ssl cert exists already")
			return existingCert.SelfLink, nil
		}
		fmt.Println("Existing SSL certificate does not match the desired certificate. Note that updating existing certificate will cause downtime.")
		if forceUpdate {
			fmt.Println("Updating existing SSL cert", name, "to match the desired state")
			return s.updateSSLCert(desiredCert)
		} else {
			fmt.Println("Will not overwrite this differing SSL cert without the --force flag.")
			return "", fmt.Errorf("will not overwrite SSL cert without --force")
		}
	}
	glog.V(5).Infof("Got error %s while trying to get existing ssl cert %s. Will try to create new one", err, name)
	// TODO: Handle non NotFound errors. We should create only if the error is NotFound.
	// Create the ssl cert.
	return s.createSSLCert(desiredCert)
}

func (s *SSLCertSyncer) DeleteSSLCert() error {
	name := s.namer.SSLCertName()
	fmt.Println("Deleting ssl cert", name)
	err := s.scp.DeleteSslCertificate(name)
	if err != nil && !utils.IsHTTPErrorCode(err, http.StatusNotFound) {
		fmt.Println("error", err, "in deleting ssl cert", name)
		return err
	}
	fmt.Println("ssl cert", name, "deleted successfully")
	return nil
}

func (s *SSLCertSyncer) updateSSLCert(desiredCert *compute.SslCertificate) (string, error) {
	name := desiredCert.Name
	fmt.Println("Deleting existing ssl cert", name, "and recreating it to match the desired state.")
	// SSL Cert does not support update. We need to delete and then create again.
	// Note: This is different than what we do in ingress-gce.
	// In ingress-gce, we first create a new cert with a different name,
	// update the target proxy to point to the new cert and then delete
	// the old cert.
	// What we do here is simpler, but can lead to downtime in the brief period
	// when we have deleted the old cert but havent created the new one.
	// TODO(nikhiljindal): Converge this with ingress-gce by sharing the same code.
	// https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/124
	err := s.scp.DeleteSslCertificate(name)
	if err != nil {
		return "", fmt.Errorf("error in deleting ssl cert %s: %s", name, err)
	}
	_, err = s.scp.CreateSslCertificate(desiredCert)
	if err != nil {
		return "", fmt.Errorf("error in creating ssl cert %s: %s", name, err)
	}
	fmt.Println("SSL cert", name, "updated successfully")
	sc, err := s.scp.GetSslCertificate(name)
	if err != nil {
		return "", err
	}
	return sc.SelfLink, nil
}

func (s *SSLCertSyncer) createSSLCert(desiredCert *compute.SslCertificate) (string, error) {
	name := desiredCert.Name
	fmt.Println("Creating ssl cert", name)
	glog.V(5).Infof("Creating ssl cert %v", desiredCert)
	_, err := s.scp.CreateSslCertificate(desiredCert)
	if err != nil {
		return "", err
	}
	fmt.Println("SSL cert", name, "created successfully")
	sc, err := s.scp.GetSslCertificate(name)
	if err != nil {
		return "", err
	}
	return sc.SelfLink, nil
}

func sslCertMatches(desiredCert, existingCert compute.SslCertificate) bool {
	// Clear output-only fields to do our comparison
	existingCert.CreationTimestamp = ""
	existingCert.Kind = ""
	existingCert.Id = 0
	existingCert.SelfLink = ""

	// NOTE: We do not print the diff, to not leak the certificate.
	return reflect.DeepEqual(existingCert, desiredCert)
}

func (s *SSLCertSyncer) desiredSSLCert(lbName string, ing *v1beta1.Ingress, client kubeclient.Interface) (*compute.SslCertificate, error) {
	// Check for secret.
	tlsLoader := &tls.TLSCertsFromSecretsLoader{Client: client}
	cert, err := tlsLoader.Load(ing)
	if err != nil {
		err = fmt.Errorf("Error in fetching certs for ing %s/%s: %s", ing.Namespace, ing.Name, err)
		fmt.Println(err)
		return nil, err
	}
	if cert == nil {
		err = fmt.Errorf("could not fetch certs for ing %s/%s", ing.Namespace, ing.Name)
		fmt.Println(err)
		return nil, err
	}
	// Compute the desired ssl cert.
	return &compute.SslCertificate{
		Name:        s.namer.SSLCertName(),
		Description: fmt.Sprintf("SSL cert for kubernetes multicluster loadbalancer %s", lbName),
		Certificate: cert.Cert,
		PrivateKey:  cert.Key,
	}, nil
}
