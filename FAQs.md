# FAQ

## What is kubemci?

kubemci is a command line tool that allows users to create multicluster ingresses
using [Google Cloud Load Balancer (GCLB)](https://cloud.google.com/load-balancing/)
It enables users to perform [cross-region load
balancing](https://cloud.google.com/compute/docs/load-balancing/http/cross-region-example) using
multiple kubernetes clusters in different Google Cloud regions.

## Does it work with other load balancer providers

It works only with Google Cloud Load Balancer for now. In future, we can
integrate with other load balancer providers, but to provide the same
cross-region load balancing as GCLB, we will need to setup Geo-DNS servers that
will route the request to the closest cluster. GCLB is able to do that using
Anycast.

## How is this different than federated ingress

Users can use kubernetes [federation](https://github.com/kubernetes/federation)
to get the same functionality.
We expect kubemci to be easier to use and more robust.

kubemci allows users to manage multicluster ingresses without having to enroll
all the clusters in a federation first.
This relieves them of the overhead of managing a federation control plane
in exchange for having to run the kubemci command explicitly each time they want
to add or remove a cluster.

Also since kubemci creates GCE resources (backend services, health checks,
forwarding rules, etc) itself it does not have the same
[problem](https://github.com/kubernetes/kubernetes/issues/36327) of ingress
controllers in each cluster fighting with each other.
In addition to the regular kubernetes CRUD operations, kubemci can also support
specific commands for ex: get-status returning the global IP and the list of
clusters that a multicluster ingress is spread to; or to run create/update in a
dry-run mode first to check the changes it will make; or to run a validate
command that validates for ex that the required services exist in all clusters
as expected (we do not have dry-run or validate commands yet, but we can add
them).

## How do I get the self-healing properties of controller model with kubemci

Users can run kubemci as a cron job so that it reads input from a data source
(such as github repository) and runs kubemci commands based on that.
Instead of a periodic job, it can also be setup as a job that watches for
changes and runs the kubemci commands.
kubemci can also be integrated with existing CI/CD pipelines to run kubemci
whenever a new cluster or multicluster ingress is created or deleted.
