#!/bin/bash

# Copyright 2017 Google Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# TODO: make set -e work?
# TODO: make set -u work?

. $(dirname ${BASH_SOURCE})/util.sh

############################################
# Change these variables to suit your setup.
#
IP="your_ip_address_here"
KUBE_CONFIG="./zpkubeconfig"
KUBECTLus="kubectl --kubeconfig=$KUBE_CONFIG --context=cluster-us-west"
KUBECTLeu="kubectl --kubeconfig=$KUBE_CONFIG --context=cluster-eu-west"
PROJECT="kubemci-demo-project"

KUBEMCI="./kubemci"
############################################

KUBECTL="kubectl --kubeconfig=$KUBE_CONFIG"

run "clear"

desc "List all clusters"
run "${KUBECTL} config get-contexts -o name"

desc "Create the zone printer app in both clusters"
run "${KUBECTLus} create -f ../examples/zone-printer/manifests/"
run "${KUBECTLeu} create -f ../examples/zone-printer/manifests/"

desc "Look at kubemci help"
run "${KUBEMCI} -h"

desc "Look at kubemci create help"
run "${KUBEMCI} create -h"

desc "Create a multicluster load balancer named zoneprinter-lb"
run "${KUBEMCI} create zoneprinter-lb --ingress=../examples/zone-printer/ingress/ingress.yaml --gcp-project=$PROJECT --kubeconfig=$KUBE_CONFIG"

desc "Get status of the load balancer"
run "${KUBEMCI} get-status zoneprinter-lb --gcp-project=$PROJECT"

# Note to demo-er: CTRL-C this command once this IP is serving the expected page.
desc "Wait for the loadbalancer setup to finish"
run "while true; do wget -O- http://$IP/; sleep 10; done"

desc "Hit the IP address from US cluster"
run "${KUBECTLus} run -i --tty busybox --image=busybox --restart=Never --rm -- wget -qO- http://$IP/"

desc "Hit the IP address from EU cluster"
run "${KUBECTLeu} run -i --tty busybox --image=busybox --restart=Never --rm -- wget -qO- http://$IP/"

desc "Delete the app from EU cluster"
run "${KUBECTLeu} delete -f ../examples/zone-printer/manifests/"

desc "Hit the IP address again from EU cluster"
run "${KUBECTLeu} run -i --tty busybox2 --image=busybox --restart=Never --rm -- wget -qO- http://$IP/"

desc "Delete multicluster load balancer"
run "${KUBEMCI} delete zoneprinter-lb --ingress=../examples/zone-printer/ingress/ingress.yaml --gcp-project=$PROJECT --kubeconfig=$KUBE_CONFIG"

desc "Delete the zone printer app from both clusters"
run "${KUBECTLus} delete -f ../examples/zone-printer/manifests/"
run "${KUBECTLeu} delete -f ../examples/zone-printer/manifests/"
