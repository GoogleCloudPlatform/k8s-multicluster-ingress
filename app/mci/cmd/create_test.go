package cmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type ExpectedCommand struct {
	Args   []string
	Output string
	Err    error
}

func run(expectedCmds []ExpectedCommand, runFn func() error) error {
	i := 0
	ExecuteCommand = func(args []string) (string, error) {
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
	// ValidateArgs should succeed when IngressFilename is set.
	options.IngressFilename = "ingress.yaml"
	if err := ValidateArgs(options, []string{}); err != nil {
		t.Errorf("unexpected error from ValidateArgs: %s", err)
	}
}

func TestRunCreate(t *testing.T) {
	options := &CreateOptions{
		IngressFilename: "ingress.yaml",
		Kubeconfig:      "kubeconfig",
	}
	runFn := func() error {
		return RunCreate(options)
	}
	expectedCommands := []ExpectedCommand{
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "config", "get-contexts", "-o=name"},
			Output: "context-1\ncontext-2",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "create", "--filename=ingress.yaml", "--context=context-1"},
			Output: "ingress created",
			Err:    nil,
		},
		{
			Args:   []string{"kubectl", "--kubeconfig=kubeconfig", "create", "--filename=ingress.yaml", "--context=context-2"},
			Output: "ingress created",
			Err:    nil,
		},
	}
	if err := run(expectedCommands, runFn); err != nil {
		t.Errorf("%s", err)
	}
}
