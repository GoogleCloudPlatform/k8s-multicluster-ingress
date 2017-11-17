# kubemci demo

This directory contains code for kubemci demo.
To try it out, follow the setup instructions detailed below and then run
`demo.sh`.

# Setup

## Create a kubeconfig

The demo script assumes that there exists a kubeconfig file called `zpkubeconfig`
in the demo folder. That kubeconfig should have 2 contexts called `cluster-eu-west`
(with a cluster in GCP EU region) and `cluster-us-west` (with a cluster in GCP US
region).

## Reserve an IP address

It also assumes that there is an enviroment variable ZP_KUBEMCI_IP set with the
name of the GCP ip address that will be used for kubemci.

You can get the list of existing GCP IP addresses in your project by running:
`gcloud compute addresses list`
You can create a new IP address by running:
`gcloud compute addresses create --global "${ZP_KUBEMCI_IP}"`

To update `ingress/nginx.yaml` to use your reserved IP address name, run:
`sed -i -e "s/\$ZP_KUBEMCI_IP/${ZP_KUBEMCI_IP}/" ../ingress/nginx.yaml`

## GCP Project

The script assumes a GCP project with the name `kubemci-demo-project`. Replace
it everywhere in `demo.sh` with your project name.
