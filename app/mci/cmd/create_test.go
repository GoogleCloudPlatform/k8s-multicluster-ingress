package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type ExpectedCommand struct {
	Args   []string
	Output string
	Err    error
}

func run(expectedCmds []ExpectedCommand, runFn func() error) error {
	getClientset = func(kubeconfigPath string) (kubeclient.Interface, error) {
		return &fake.Clientset{}, nil
	}

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
	err := runFn()
	if err != nil {
		return err
	}
	if i != len(expectedCmds) {
		return fmt.Errorf("expected commands not called: %s", expectedCmds[i:])
	}
	return nil
}

func TestValidateArgs(t *testing.T) {
	// ValidateArgs should return an error with empty options.
	options := &CreateOptions{}
	if err := ValidateArgs(options, []string{}); err == nil {
		t.Errorf("Expected error for missing ingress")
	}
	// ValidateArgs should return an error with missing load balancer name.
	options.IngressFilename = "ingress.yaml"
	if err := ValidateArgs(options, []string{}); err == nil {
		t.Errorf("Expected error for missing load balancer name")
	}

	// ValidateArgs should succeed when IngressFilename is set.
	options.IngressFilename = "ingress.yaml"
	if err := ValidateArgs(options, []string{"lbname"}); err != nil {
		t.Errorf("unexpected error from ValidateArgs: %s", err)
	}
}

func TestRunCreate(t *testing.T) {
	options := &CreateOptions{
		IngressFilename: "../../../testdata/ingress.yaml",
		Kubeconfig:      "kubeconfig",
	}
	runFn := func() error {
		return RunCreate(options, []string{"lbname"})
	}
	expectedCommands := []ExpectedCommand{
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
			Output: "context-1\ncontext-2",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "create", "--filename=../../../testdata/ingress.yaml", "--context=context-1"},
			Output: "ingress created",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "create", "--filename=../../../testdata/ingress.yaml", "--context=context-2"},
			Output: "ingress created",
			Err:    nil,
		},
	}
	if err := run(expectedCommands, runFn); err != nil {
		t.Errorf("%s", err)
	}
}
