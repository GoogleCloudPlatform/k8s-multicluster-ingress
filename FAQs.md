# FAQ

## What is kubemci?

kubemci is a command line tool to manage ingresses with multiple kubernetes clusters.
In its first version, it enables users to use [Google Cloud Load Balancer
(GCLB)](https://cloud.google.com/load-balancing/) to perform [cross-region load
balancing](https://cloud.google.com/compute/docs/load-balancing/http/cross-region-example) using
multiple kubernetes clusters in different Google Cloud regions.

## Which load balancer providers does it work with?

It works only with Google Cloud Load Balancer for now, and we want to integrate
with other load balancer providers in the future.
GCLB provides geo-awareness using anycast. To provide the same functionality
with other load balancer providers, we will need to setup Geo-DNS servers or
require them to support anycast as well.

## When do I need to update multicluster ingress after kubemci create?

### Adding/removing an existing multicluster ingress from a cluster.

To add an existing multicluster ingress to a new cluster, users can
run `kubemci create` again with the updated list of clusters.
To remove an exising multicluster ingress from a cluster, users need to delete
the ingress and create it again using `kubemci create`.

### When a cluster goes down or the pods/services in that cluster go down.

When a cluster goes down or the pods or services in that cluster go down, GCLB
health checks will start failing and hence GCLB will automatically stop sending
traffic to that cluster/pods. Users do not need to run any kubemci command unless
they want to permanently remove the multicluster ingress from that cluster.

### When the ingress spec changes

When the ingress spec changes, users need to run `kubemci create` again with the
updated spec to update GCLB.
To automate this, users can run kubemci as a cron job that reads input from a data source
(such as github repository) and runs kubemci commands based on that.
Instead of a periodic job, users can also set it up as a job that watches for
ingress spec changes and runs the `kubemci create` command.
kubemci can also be integrated with existing CI/CD pipelines to run kubemci
whenever ingress spec is updated.

### Deleting the multicluster ingress

Users can run `kubemci delete` to delete an existing multicluster ingress.

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
forwarding rules, etc) itself, it does not have the same
[problem](https://github.com/kubernetes/kubernetes/issues/36327) of ingress
controllers in each cluster competing with each other to program similar
resources.
In addition to the regular kubernetes CRUD operations, kubemci can also support
specific commands for ex: get-status returning the global IP and the list of
clusters that a multicluster ingress is spread to; or to run create/update in a
dry-run mode first to check the changes it will make; or to run a validate
command that validates for ex that the required services exist in all clusters
as expected (we do not have dry-run or validate commands yet, but we have plans
to add them).
