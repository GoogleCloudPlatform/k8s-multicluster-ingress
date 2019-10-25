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

// SyncerInterface is an interface to manage GCP ssl certs.
type SyncerInterface interface {
	// EnsureSSLCerts ensures that the required ssl cert exists for the given ingress.
	// Will only change existing SSL certs if forceUpdate=True.
	// Returns the self link for the ensured ssl cert.
	EnsureSSLCerts(lbName string, ing *v1beta1.Ingress, client kubeclient.Interface, forceUpdate bool) ([]string, error)
	// DeleteSSLCerts deletes the ssl certs that EnsureSSLCerts would have created.
	DeleteSSLCerts() error
}
