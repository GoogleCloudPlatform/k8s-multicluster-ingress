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

package kubeutils

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

var fakeError = fmt.Errorf("fake error")

type expectedCommand struct {
	Args   []string
	Output string
	Err    error
}

func runGetClusterContexts(kubeContexts []string, expectedCmds []expectedCommand) ([]string, error) {
	i := 0
	executeCommand = func(args []string) (string, error) {
		if i >= len(expectedCmds) {
			return "", fmt.Errorf("unexpected command: %s", strings.Join(args, " "))
		}
		if !reflect.DeepEqual(args, expectedCmds[i].Args) {
			return "", fmt.Errorf("unexpected command: %s, was expecting   : %s", strings.Join(args, " "), strings.Join(expectedCmds[i].Args, " "))
		}
		output, err := expectedCmds[i].Output, expectedCmds[i].Err
		i++
		return output, err
	}
	clusters, err := getClusterContexts("kubeconfig", kubeContexts)
	if err != nil {
		return clusters, err
	}
	if i != len(expectedCmds) {
		return clusters, fmt.Errorf("expected [commands, outputs, errs] not called: %s", expectedCmds[i:])
	}
	return clusters, nil
}

func TestGetClusterContexts(t *testing.T) {
	testCases := []struct {
		kubeContexts     []string
		expectedCmds     []expectedCommand
		expectedClusters []string
		expectedErr      bool
	}{
		{
			// Should return all the contexts from kubeconfig.
			[]string{},
			[]expectedCommand{
				{
					[]string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
					"cluster1\ncluster2",
					nil,
				},
			},
			[]string{"cluster1", "cluster2"},
			false,
		},
		{
			// Returns error if kubeconfig get contexts fails.
			[]string{},
			[]expectedCommand{
				{
					[]string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
					"error",
					fakeError,
				},
			},
			[]string{},
			true,
		},
		{
			// Returns the given kubeContexts if valid.
			[]string{"cluster1"},
			[]expectedCommand{
				{
					[]string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name", "cluster1"},
					"cluster1",
					nil,
				},
			},
			[]string{"cluster1"},
			false,
		},
		{
			// Returns an error if given kubeContexts is invalid.
			[]string{"cluster3"},
			[]expectedCommand{
				{
					[]string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name", "cluster3"},
					"error",
					fakeError,
				},
			},
			[]string{},
			true,
		},
	}
	for index, c := range testCases {
		clusters, err := runGetClusterContexts(c.kubeContexts, c.expectedCmds)
		if c.expectedErr != (err != nil) {
			t.Errorf("case %d: unexpected error, expected err != nil: %v, got err: %s", index, c.expectedErr, err)
		}
		if err != nil {
			continue
		}
		// Sort the list of clusters before comparing.
		sort.Strings(clusters)
		if !reflect.DeepEqual(clusters, c.expectedClusters) {
			t.Errorf("case %d: unexpected list of clusters in which ingress was created. expected: %v, got: %v", index, c.expectedClusters, clusters)
		}
	}
}

func TestGetClientsForContexts(t *testing.T) {
	testCases := []struct {
		kubeContexts    []string
		getClientset    func(string, string) (kubeclient.Interface, error)
		expectedClients map[string]kubeclient.Interface
		expectedErr     bool
	}{
		{
			// Should return a client for each given kubecontext.
			[]string{"cluster1", "cluster2"},
			func(string, string) (kubeclient.Interface, error) {
				return &fake.Clientset{}, nil
			},
			map[string]kubeclient.Interface{
				"cluster1": &fake.Clientset{},
				"cluster2": &fake.Clientset{},
			},
			false,
		},
		{
			// Should return an empty map for empty kubecontexts.
			[]string{},
			func(string, string) (kubeclient.Interface, error) {
				return &fake.Clientset{}, nil
			},
			map[string]kubeclient.Interface{},
			false,
		},
		{
			// Should return an error if getClientset returns an error.
			[]string{"cluster1", "cluster2"},
			func(string, string) (kubeclient.Interface, error) {
				return nil, fakeError
			},
			map[string]kubeclient.Interface{},
			true,
		},
	}
	for index, c := range testCases {
		getClientset = c.getClientset
		clients, err := getClientsForContexts("kubeconfig", c.kubeContexts)
		if c.expectedErr != (err != nil) {
			t.Errorf("case %d: unexpected error, expected err != nil: %v, got err: %s", index, c.expectedErr, err)
		}
		if err != nil {
			continue
		}

		if len(clients) != len(c.expectedClients) {
			t.Errorf("unexpected set of clients, expected: %v, got: %v", c.expectedClients, clients)
		}
		for k, _ := range c.expectedClients {
			if clients[k] == nil {
				t.Errorf("unexpected set of clients, expected: %v, got: %v", c.expectedClients, clients)
			}
		}
	}
}
