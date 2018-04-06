package e2e

import (
	"fmt"

	"github.com/golang/glog"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/cloudinterface"
	"github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/loadbalancer"
	utilsnamer "github.com/GoogleCloudPlatform/k8s-multicluster-ingress/app/kubemci/pkg/gcp/namer"
)

const (
	svcNodePort    = 30061
	expectedHcPath = "/healthz"
)

func RunHCFromProbeTest() {
	project, kubeConfigPath, lbName, ipName, clients := initDeps()
	kubectlArgs := []string{"kubectl", fmt.Sprintf("--kubeconfig=%s", kubeConfigPath)}
	setupHCFromProbe(kubectlArgs, ipName, clients)
	defer func() {
		cleanupHCFromProbe(kubectlArgs, ipName, clients)
		cleanupDeps(ipName)
	}()

	testHCFromProbe(project, kubeConfigPath, lbName)
}

func setupHCFromProbe(kubectlArgs []string, ipName string, clients map[string]kubeclient.Interface) {
	deployApp(kubectlArgs, clients, "testdata/e2e/hcFromProbe/app/")
	replaceVariable("testdata/e2e/hcFromProbe/ingress.yaml", "\\$E2E_KUBEMCI_IP", ipName)
}

func cleanupHCFromProbe(kubectlArgs []string, ipName string, clients map[string]kubeclient.Interface) {
	cleanupApp(kubectlArgs, clients, "testdata/e2e/hcFromProbe/app/")
	replaceVariable("testdata/e2e/hcFromProbe/ingress.yaml", ipName, "\\$E2E_KUBEMCI_IP")
}

func testHCFromProbe(project, kubeConfigPath, lbName string) {
	deleteFn, err := createIngress(project, kubeConfigPath, lbName, "testdata/e2e/hcFromProbe/ingress.yaml")
	if err != nil {
		glog.Fatalf("error creating ingress: %+v", err)
		return
	}
	defer deleteFn()

	// Tests
	ipAddress := getIpAddress(project, lbName)
	// Ensure that the IP address eventually returns 200 (on health check path, since the service used returns 404 on /)
	if err := waitForIngress("http", ipAddress, expectedHcPath); err != nil {
		glog.Errorf("error in waiting for ingress: %s", err)
	}

	cloud, err := cloudinterface.NewGCECloudInterface(project)
	if err != nil {
		glog.Errorf("error in creating cloud interface: %s\n", err)
		return
	}

	namer := utilsnamer.NewNamer(loadbalancer.MciPrefix, lbName)
	hc, err := cloud.GetHealthCheck(namer.HealthCheckName(svcNodePort))
	if err != nil {
		glog.Errorf("error in getting healthcheck: %s\n", err)
		return
	}

	if hcPath := hc.HttpHealthCheck.RequestPath; hcPath != expectedHcPath {
		glog.Errorf("error health check path '%s' does not match readiness probe path '%s'\n", hcPath, expectedHcPath)
		return
	}
	fmt.Println("PASS: health check path matches path from readiness probe")
	//TODO add tests for timeout, checkInterval etc once that has been implemented
}
