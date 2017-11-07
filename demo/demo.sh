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

. $(dirname ${BASH_SOURCE})/util.sh

KUBECTL="kubectl --kubeconfig=zpkubeconfig"
KUBECTLus="kubectl --kubeconfig=zpkubeconfig --context=cluster-us-west"
KUBECTLeu="kubectl --kubeconfig=zpkubeconfig --context=cluster-eu-west"
KUBEMCI="kubemci"

run "clear"

desc "List all clusters"
run "${KUBECTL} config get-contexts -o name"

desc "Create the zone printer app in both clusters"
run "${KUBECTLus} create -f ../examples/zone-printer/app/"
run "${KUBECTLeu} create -f ../examples/zone-printer/app/"

desc "Look at kubemci help"
run "${KUBEMCI} -h"

desc "Look at kubemci create help"
run "${KUBEMCI} create -h"

desc "Create a multicluster load balancer zoneprinter-lb"
run "${KUBEMCI} create zoneprinter-lb --ingress=../examples/zone-printer/ingress/nginx.yaml --gcp-project=kubemci-demo-project --kubeconfig=zpkubeconfig"

desc "Get status of the load balancer"
run "${KUBEMCI} get-status zoneprinter-lb --gcp-project=kubemci-demo-project"

desc "Hit the IP address from US cluster"
run "${KUBECTLus} run -i --tty busybox --image=busybox --restart=Never -- wget -qO- http://35.190.81.146/"

desc "Hit the IP address from EU cluster"
run "${KUBECTLeu} run -i --tty busybox --image=busybox --restart=Never -- wget -qO- http://35.190.81.146/"

desc "Delete the app from EU cluster"
run "${KUBECTLeu} delete -f ../examples/zone-printer/app/"

desc "Hit the IP address again from EU cluster"
run "${KUBECTLeu} run -i --tty busybox2 --image=busybox --restart=Never -- wget -qO- http://35.190.81.146/"

desc "Delete multicluster load balancer"
run "${KUBEMCI} delete zoneprinter-lb --ingress=../examples/zone-printer/ingress/nginx.yaml --gcp-project=kubemci-demo-project --kubeconfig=zpkubeconfig"

desc "Delete the zone printer app from both clusters"
run "${KUBECTLus} delete -f ../examples/zone-printer/app/"
run "${KUBECTLeu} delete -f ../examples/zone-printer/app/"
run "${KUBECTLus} delete pods busybox"
run "${KUBECTLeu} delete pods busybox"
run "${KUBECTLeu} delete pods busybox2"
