# kubemci

[![GoReportCard Widget]][GoReportCard] [![Coveralls Widget]][Coveralls] [![GoDoc Widget]][GoDoc] [![Slack Widget]][Slack]

[GoReportCard Widget]: https://goreportcard.com/badge/github.com/GoogleCloudPlatform/k8s-multicluster-ingress
[GoReportCard]: https://goreportcard.com/report/github.com/GoogleCloudPlatform/k8s-multicluster-ingress
[Coveralls Widget]: https://coveralls.io/repos/github/GoogleCloudPlatform/k8s-multicluster-ingress/badge.svg
[Coveralls]: https://coveralls.io/github/GoogleCloudPlatform/k8s-multicluster-ingress
[GoDoc Widget]: https://godoc.org/github.com/GoogleCloudPlatform/k8s-multicluster-ingress?status.svg
[GoDoc]: https://godoc.org/github.com/GoogleCloudPlatform/k8s-multicluster-ingress
[Slack Widget]: https://s3.eu-central-1.amazonaws.com/ngtuna/join-us-on-slack.png
[Slack]: http://slack.kubernetes.io#sig-multicluster

kubemci is a tool to configure Kubernetes ingress to load balance traffic across
multiple Kubernetes clusters.

This is a Google Cloud Platform [beta](https://cloud.google.com/terms/launch-stages) tool, suitable for limited production use cases:
https://cloud.google.com/kubernetes-engine/docs/how-to/setup-multi-cluster-ingress

## Getting started

You can try out kubemci using the [zone printer example](/examples/zone-printer).

Follow the instructions as detailed [here](/examples/zone-printer/README.md).

To create an HTTPS ingress, follow the instructions [here](/examples/zone-printer/https.md).

## Authorization

By default, kubemci relies on the discovery of [Application Default Credentials](https://cloud.google.com/docs/authentication/production#finding_credentials_automatically) to authorize access to GCP resources.

As an alternative, kubemci accepts an `--access-token` argument that takes an oauth access token,
such as one generated for a service account via `gcloud --impersonate-service-account ... auth print-access-token`.
This method obviates the need to distribute private keys.

## More information

We have a [video](https://www.youtube.com/watch?v=0_Yt_1yICfk) explaining what
kubemci is intended for. It also shows a demo of setting up a multicluster
ingress.

We also have an [FAQ](/FAQs.md) for common questions.

## Contributing

See [CONTRIBUTING.md](/CONTRIBUTING.md) for instructions on how to contribute.

You can also checkout [existing
issues](https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues) for ways to contribute.

## Feedback

If you are using kubemci, we would love to hear from you! Tell us how you are
using it and what works and what does not:
https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/117

## Caveats

* Users will be need to specify a unique NodePort for their multicluster services (that should be available across all clusters). This is a pretty onerous requirement, required because health checks need to be the same across all clusters.

* This will only work for clusters in the same GCP project. In future, we can integrate with Shared VPC to enable cross project load balancing.

* Load balancing across clusters in the same region will happen in proportion to the number of nodes in each cluster, instead of number of containers.

* Since ILBs and ingress share the same instance groups (IGs), there is a race condition where deleting ILBs can cause the IG supposed to be used for multicluster ingress to be deleted. This will be fixed in the next ingress controller forced sync (every 10 mins). The same race condition exists in single cluster ingress as well.

* Users need to explicitly update all their existing multicluster ingresses (by running `kubemci create ingress`), if they add nodes from a new zone to a cluster. This is required so that the tool can update backend service and add a new instance group to it.
