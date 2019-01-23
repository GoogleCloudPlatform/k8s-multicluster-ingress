# Demo: Zone Printer

Zone Printer is a web application that displays the
[Google Cloud Platform (GCP) compute zone](https://cloud.google.com/compute/docs/regions-zones/)
where the application is running.

## How does it work?

Zone Printer application is a web server written in Go. It greets the user
with the country name, flag and the zone of the server it is deployed in.

It queries the [GCE metadata
server](https://cloud.google.com/compute/docs/storing-retrieving-metadata) to
find out the zone name it is running on. Since GKE nodes are GCE instances, this
application will print the name of the zone it is running on.

The `Deployment` consists of a container image that is listening on port `80`.
The Service exposes the `Deployment` pods on `nodePort: 30061` on all the nodes
in the Kubernetes cluster.

The example also includes an `Ingress` manifest (`ingress/ingress.yaml`) that is
used by `kubemci` to provision the multi-cluster Ingress.

## Step 0: Before you begin

1. **GCP Project**

    Ensure that you have a GCP Project. If you don't have one already,
    you can create a project by following the instructions at
    [Creating and Managing Projects](https://cloud.google.com/resource-manager/docs/creating-managing-projects).

    After your project is created, run the following commands in your
    terminal:

    ```shell
    PROJECT=<your-project-name>
    ```

2. **Set up `gcloud`**:

    You can install `gcloud` by following the instructions at
    [Installing Cloud SDK](https://cloud.google.com/sdk/downloads) and
    initialize it by running the following commands:

    ```shell
    gcloud init # Set $PROJECT as the default project
    gcloud auth login
    gcloud auth application-default login
    ```

3. Set up **`kubectl`**:

    You can install `kubectl` by running:

    ```shell
    gcloud components install kubectl
    ```

4. Install **`kubemci`**:

    [kubemci](https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress) is the primary command-line tool used to create and configure a multi-cluster ingress.

    Download one of the binaries and place it in an executable path:

    - [Linux](https://storage.googleapis.com/kubemci-release/release/latest/bin/linux/amd64/kubemci)
    - [macOS](https://storage.googleapis.com/kubemci-release/release/latest/bin/darwin/amd64/kubemci)

    
    For example on linux you can install `kubemci` by running:

    ```shell
    wget https://storage.googleapis.com/kubemci-release/release/latest/bin/linux/amd64/kubemci
    ```

    Ensure that the binary is executable. This can be done in the directory of the binary with the following command:

    ```shell
    chmod +x ./kubemci
    ```

    In google cloud shell you can move this to your local bin to automatically include it in your path

    ```shell
    mv ./kubemci ~/bin/kubemci
    ```

## 1. Create Kubernetes Clusters

`kubemci` requires Kubernetes clusters that are v1.8.1 or newer. (You can check
your cluster version using the `kubectl version` command.)

You will need at least two Kubernetes clusters in two different compute [zones]
to verify that `kubemci` works.

[zones]: https://cloud.google.com/compute/docs/regions-zones/

#### Creating new GKE clusters

The following commands will create two clusters:

- `cluster-1` in `us-east4-a`
- `cluster-2` in `europe-west1-c`

and save their connection details to a file named `clusters.yaml`.

```shell
# Create a cluster in us-east and get its credentials
KUBECONFIG=clusters.yaml gcloud container clusters create \
    --cluster-version=1.9 \
    --zone=us-east4-a \
    cluster-1

# Create a cluster in eu-west and get its credentials
KUBECONFIG=clusters.yaml gcloud container clusters create \
    --cluster-version=1.9 \
    --zone=europe-west1-c \
    cluster-2
```

#### Using existing GKE clusters

> **Note:** If you don't already have Kubernetes clusters on GCP, skip to the
next subsection on [creating new clusters](#creating-new-gke-clusters).

If you already have existing Kubernetes Engine clusters, run `get-credentials`
with them to create a `clusters.yaml` file with the connection information of
these clusters.

For example:

```shell
KUBECONFIG=clusters.yaml gcloud container clusters \
    get-credentials cluster-1 --zone=us-east4-a

KUBECONFIG=clusters.yaml gcloud container clusters \
    get-credentials cluster-2 --zone=europe-west1-c

# ...repeat for other clusters you would like to add to the Ingress
```

#### Using non-GKE clusters on GCE

It is possible to use Kubernetes clusters hosted on Google Compute Engine (GCE)
with `kubemci`. You need to create a kubeconfig file that contains the
connection information of the clusters you want to add to the multi-cluster Ingress.

First, make sure you have the kubeconfig entries of the clusters:

```shell
$ kubectl config get-contexts -o name

name-of-cluster-1
name-of-cluster-2
[...]
```

For each cluster you will use, export its configuration into a YAML file:

```shell
# do this for the cluster-1 in us-east:
kubectl config use-context [name-of-cluster-1]
kubectl config view --minify --flatten > cluster-1.yaml

# do this for the cluster-2 in eu-west:
kubectl config use-context [name-of-cluster-2]
kubectl config view --minify --flatten > cluster-2.yaml
```

Combine these `cluster-1.yaml` and `cluster-2.yaml` config files into one called `clusters.yaml`:

```shell
KUBECONFIG=cluster-1.yaml:cluster-2.yaml kubectl config view \
  --flatten > clusters.yaml
```

## Step 2: Deploy the sample application

Deploy the application along with its NodePort Service in each of the two
clusters. You can get the cluster contexts from `kubectl config get-contexts`
and iterate through all the clusters to deploy the [application
manifests](./manifests/). This could be accomplished by running the following loop:

```shell
for ctx in $(kubectl config get-contexts -o=name --kubeconfig clusters.yaml); do
  kubectl --kubeconfig clusters.yaml --context="${ctx}" create -f manifests/
done
```

## Step 3: Reserve a static IP address

This command reserves a static IP address for the load balancer.

```shell
ZP_KUBEMCI_IP="zp-kubemci-ip"
gcloud compute addresses create --global "${ZP_KUBEMCI_IP}"
```

## Step 4: Use the static IP on Ingress manifest

Replace the value of `kubernetes.io/ingress.global-static-ip-name`
annotation in `ingress/ingress.yaml` with the value of `$ZP_KUBEMCI_IP`.

```shell
sed -i -e "s/\$ZP_KUBEMCI_IP/${ZP_KUBEMCI_IP}/" ingress/ingress.yaml
```

## Step 5: Deploy the multi-cluster Ingress with `kubemci`

Run `kubemci` to deploy the [ingress/ingress.yaml](./ingress/ingress.yaml) and
create the multi-cluster Ingress named `zone-printer`:

```shell
kubemci create zone-printer \
    --ingress=ingress/ingress.yaml \
    --gcp-project=$PROJECT \
    --kubeconfig=clusters.yaml
```

This command creates the multi-cluster Ingress.

## Step 6: View multi-cluster Ingress status

You can view the status of the multi-cluster Ingress you just created using the
`kubemci get-status` command:

```shell
$ kubemci get-status zone-printer --gcp-project=$PROJECT

Load balancer zone-printer has IPAddress 35.190.74.228 and
is spread across 2 clusters (gke_yourproject_us-east4-a_cluster-1,
gke_yourproject_europe-west1-c_cluster-2).
```

## Step 7: Test the multi-cluster Ingress

It will take several minutes from when the Ingress and Google Cloud HTTP Load
Balancer are created until they are ready. (You may see HTTP 404 or HTTP 502
errors while the load balancer is getting ready.)

Once the load balancer is ready to serve traffic, use the IP address printed
in the previous step and query the server:

```shell
$ curl http://35.190.74.228

Welcome to the global site! You are being served from us-central1-b.
Congratulations
```

Multi-cluster Ingress has routed your traffic to a cluster in the nearest
GCP region.

## Step 8: Cleanup

To delete the multi-cluster Ingress you created, run:

```shell
kubemci delete zone-printer \
    --ingress=ingress/ingress.yaml \
    --gcp-project=$PROJECT \
    --kubeconfig=clusters.yaml
```

This will also clean up the underlying networking resources provisioned by
`kubemci`.

If you created new Kubernetes Engine clusters for this tutorial, clean them up:

```shell
gcloud container clusters delete cluster-1 --zone=us-east4-a -q

gcloud container clusters delete cluster-2 --zone=europe-west1-c -q
```
