#!/bin/bash

# Copyright 2018 Google Inc.
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

set -e
set -u
set -o pipefail

files=$(find . -name "*.go"| grep -v "/vendor/")
PKG=github.com/GoogleCloudPlatform/k8s-multicluster-ingress
files=$(go list -f '{{if len .TestGoFiles}}"{{.Dir}}/..."{{end}}' $(go list ${PKG}/... | grep -v vendor/))
#echo files: $files
# Suggestions for changing Http to HTTP are at 0.8: Bypass those using confidence >.8
diff=$(echo $files | xargs -L 1 golint -min_confidence=0.81)
if [ -n "${diff}" ]; then
  echo -e "golint failed:\n${diff}"
  exit 1
fi
