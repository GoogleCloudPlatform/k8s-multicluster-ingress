# kubemci demo

This directory contains code for kubemci demo.
To try it out, download/build `kubemci`, follow the setup instructions detailed
below and then run `demo.sh`. After creating the multicluster ingress, the
script will keep connecting to its IP address until it receives a CTRL-C. This
is to show when the loadbalancer has been fully setup. (TODO: automatically
detect when the loadbalancer is fully setup.)

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

To update `ingress/ingress.yaml` to use your reserved IP address name, run:
`sed -i -e "s/\$ZP_KUBEMCI_IP/${ZP_KUBEMCI_IP}/" ../ingress/ingress.yaml`

Edit `demo.sh` to include your IP address in the `IP` variable.

## GCP Project

The script assumes a GCP project with the name `kubemci-demo-project`. Replace
`PROJECT` in `demo.sh` with your project name.
