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


PKG=github.com/GoogleCloudPlatform/k8s-multicluster-ingress

# So that make test is not confused with the test directory.
.PHONY: test

fmt:
	@echo "+ $@"
	@go list -f '{{if len .TestGoFiles}}"gofmt -s -d {{.Dir}}"{{end}}' $(shell go list ${PKG}/... | grep -v vendor) | xargs -L 1 sh -c

lint:
	@echo "+ $@"
	@go list -f '{{if len .TestGoFiles}}"golint {{.Dir}}/..."{{end}}' $(shell go list ${PKG}/... | grep -v vendor) | xargs -L 1 sh -c

vet:
	@echo "+ $@"
	@go vet $(shell go list ${PKG}/... | grep -v vendor)

test:
	@echo "+ $@"
	@go test -race -tags "cgo" $(shell go list ${PKG}/... | grep -v vendor)

test-all: fmt lint vet test

cover:
	@echo "+ $@"
	@go list -f '{{if len .TestGoFiles}}"go test -coverprofile={{.Dir}}/.coverprofile {{.ImportPath}}"{{end}}' $(shell go list ${PKG}/... | grep -v vendor) | xargs -L 1 sh -c

coveralls:
	@echo "+ $@"
# Make sure goveralls is installed.
	@go get github.com/mattn/goveralls
	@CI_NAME="prow" CI_BUILD_NUMBER=${BUILD_ID} CI_BRANCH=${PULL_BASE_REF} CI_PULL_REQUEST=${PULL_NUMBER} goveralls -repotoken $(shell cat /etc/coveralls-token/coveralls.txt)

build:
	go build -a -installsuffix cgo ${GOPATH}/src/${PKG}/cmd/kubemci/kubemci.go
