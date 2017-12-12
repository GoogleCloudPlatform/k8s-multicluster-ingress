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
	"k8s.io/api/extensions/v1beta1"

	kubeclient "k8s.io/client-go/kubernetes"
)

const (
	FakeSSLCertSelfLink = "target/cert/self/link"
)

type FakeSSLCert struct {
	LBName  string
	Ingress *v1beta1.Ingress
}

type FakeSSLCertSyncer struct {
	// List of ssl certs that this has been asked to ensure.
	EnsuredSSLCerts []FakeSSLCert
}

// Fake ssl cert syncer to be used for tests.
func NewFakeSSLCertSyncer() SSLCertSyncerInterface {
	return &FakeSSLCertSyncer{}
}

// Ensure this implements SSLCertSyncerInterface.
var _ SSLCertSyncerInterface = &FakeSSLCertSyncer{}

func (f *FakeSSLCertSyncer) EnsureSSLCert(lbName string, ing *v1beta1.Ingress, client kubeclient.Interface, forceUpdate bool) (string, error) {
	f.EnsuredSSLCerts = append(f.EnsuredSSLCerts, FakeSSLCert{
		LBName:  lbName,
		Ingress: ing,
	})
	return FakeSSLCertSelfLink, nil
}

func (f *FakeSSLCertSyncer) DeleteSSLCert() error {
	f.EnsuredSSLCerts = nil
	return nil
}
