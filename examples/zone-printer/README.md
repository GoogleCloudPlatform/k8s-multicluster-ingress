# Zone Printer

Zone printer is an application that displays the
[Google Cloud Platform (GCP) zone](https://cloud.google.com/compute/docs/regions-zones/)
where the application is running. 

# How does it work?

> **Note:** This section explains how the application is setup. Feel free to skip to the next section if you don't care about these
details. 

Zone printer is intended to be run in a Kubernetes cluster and is
composed of: a ConfigMap (`app/zonefetch.yaml`), a Deployment
(`app/nginx-dep.yaml`) and a Service (`app/nginx-svc.yaml`).

The ConfigMap consists of a script that fetches GCP zone
information from the
[GCE metadata server](https://cloud.google.com/compute/docs/storing-retrieving-metadata). The Deployment consists of a pod
spec with two containers. One container mounts the aforementioned
ConfigMap, executes the script in the ConfigMap and writes the
output to `/usr/share/nginx/html/index.html`. The other container
runs a standard nginx server that reads from `/usr/share/nginx/html/`.

The pod, therefore, exposes the fetched zone information at
`/` or `/index.html` HTTP paths.

The Service exposes the Deployment pods on port `30061` on all the
nodes in its Kubernetes cluster.

The example also includes an Ingress manifest (`ingress/nginx.yaml`)
that will be used by `kubemci`.

# Tutorial

## 0. Prerequisite

### 0.1 GCP Project

Ensure you that you have a GCP Project. If you don't have one already,
you can create a project by following the instructions at
[Creating and Managing Projects](https://cloud.google.com/resource-manager/docs/creating-managing-projects).

After your project is created, run the following command in your
terminal:

```shell
PROJECT=<your-project-name>
```

### 0.2 `gcloud` installed and initialized

You can install `gcloud` by following the instructions at
[Installing Cloud SDK](https://cloud.google.com/sdk/downloads) and
initialize it by running the following commands:

```shell
gcloud init # Set $PROJECT as the default project
gcloud auth login
gcloud auth application-default login
```

### 0.3 kubectl

You can install `kubectl` by running:

```shell
gcloud components install kubectl
```

### 0.4 Kubernetes clusters on GCP

#### Existing clusters

> **Note:**: If you don't already have Kubernetes cluster on GCP,
skip to the next subsection on [creating new clusters](#creating-new-clusters).

`kubemci` requires Kubernetes clusters that are v1.8.1 or newer. Please
ensure that all the clusters you are using in this tutorial satisfy the
minimum version requirement. You can check your cluster version using
the `kubectl version` command.

If you already have clusters with their credentials in your local
kubeconfig (`$HOME/.kube/config` by default) file, you need to ensure
that you either only have clusters you are going to use for this
tutorial in your default kubeconfig file or create a new kubeconfig
file that just contains credentials for those clusters. Assuming that
the clusters you are going to use for this tutorial are called
`us-east`, `eu-west` and `asia-east`.

List the contexts available (note the names of the ones you wish to
use with kubemci)

```shell
$ kubectl config get-contexts -o name
us-east
eu-west
asia-east
i-dont-care
doesnt-exist
```

Now iterate over the contexts you identified, making it the context
in-use and then extracting that context to its own file.

```shell
# First, do this for the us-east one.
kubectl config use-context us-east
kubectl config view  --minify --flatten > mciuseast

# Second, do this for the eu-west one.
kubectl config use-context eu-west
kubectl config view --minify --flatten > mcieuwest

# Finally, do this for the asia-east one.
kubectl config use-context asia-east
kubectl config view --minify --flatten > mciasiaeast
```

Combine (flatten) all of these extracted contexts to create a
single kubeconfig file with just the contexts we care about

```shell
# Use KUBECONFIG to specify the context filenames we created in
# the previous step
KUBECONFIG=mciuseast:mcieuwest:mciasiaeast kubectl config view \
  --flatten > zpkubeconfig
```

#### Creating new clusters

You need at least two Kubernetes clusters in two different GCP zones
to verify that `kubemci` works. Clusters must be v1.8.1 or newer.
Let's create three GKE clusters and get their credentials for the
purposes of this tutorial:

```shell
# Create a cluster in us-east and get its credentials
gcloud container clusters create \
    --cluster-version=1.8.1-gke.0 \
    --zone=us-east4-a \
    cluster-us-east

KUBECONFIG=./zpkubeconfig gcloud container clusters get-credentials \
    --zone=us-east4-a \
    cluster-us-east


# Create a cluster in eu-west and get its credentials
gcloud container clusters create \
    --cluster-version=1.8.1-gke.0 \
    --zone=europe-west1-c \
    cluster-eu-west

KUBECONFIG=./zpkubeconfig gcloud container clusters get-credentials \
    --zone=europe-west1-c \
    cluster-eu-west


# Create a cluster in asia-east and get its credentials
gcloud container clusters create \
    --cluster-version=1.8.1-gke.0 \
    --zone=asia-east1-b \
    cluster-asia-east

KUBECONFIG=./zpkubeconfig gcloud container clusters get-credentials \
    --zone=asia-east1-b \
    cluster-asia-east
```

## 1. Deploy the application

Deploy the application along with its NodePort service in each of the
three clusters. You can get the cluster contexts from `kubectl config get-contexts` and iterate through all the clusters to deploy the
application manifests. This could be accomplished by running the
following loop:

```shell
for ctx in $(kubectl config get-contexts --kubeconfig=./zpkubeconfig -o name); do
  kubectl --context="${ctx}" create -f app/
done
```

## 2. Reserve a static IP on GCP.

We need to reserve a static IP on GCP for kubemci. Run the following
command to reserve the IP.

```shell
ZP_KUBEMCI_IP="zp-kubemci-ip" gcloud compute addresses create \
    --global "${ZP_KUBEMCI_IP}"
```

## 3. Modify the Ingress manifest.

Replace the value of `kubernetes.io/ingress.global-static-ip-name`
annotation in `ingress/nginx.yaml` with the value of $ZP_KUBEMCI_IP.

```shell
sed -e "s/\$ZP_KUBEMCI_IP/${ZP_KUBEMCI_IP}/" ingress/nginx.yaml
```

## 4. Deploy the Multi-Cluster Ingress (with kubemci)

Run kubemci to create the multi-cluster ingress.

```shell
kubemci create zone-printer \
    --ingress=ingress/nginx.yaml \
    --gcp-project=$PROJECT \
    --kubeconfig=./zpkubeconfig
```

Voila! You should have a multi-cluster ingress once the command
successfully exits!


## 5. Get the status of Multi-Cluster Ingress

You can get the status of the multi-cluster ingress you just created
by using `kubemci get-status` command.

```shell
kubemci get-status zone-printer --gcp-project=$PROJECT
```

## 6. Clean up

Delete the multi-cluster ingress by using the `kubemci delete` command.

```shell
kubemci delete zone-printer \
    --ingress=ingress/nginx.yaml \
    --gcp-project=$PROJECT \
    --kubeconfig=./zpkubeconfig
```

Delete the GKE clusters if you created them earlier in this tutorial
and you don't need them any more.

```shell
gcloud container clusters delete \
    --zone=us-east4-a \
    cluster-us-east

gcloud container clusters delete \
    --zone=europe-west1-c \
    cluster-eu-west

gcloud container clusters delete \
    --zone=asia-east1-b \
    cluster-asia-east
```
