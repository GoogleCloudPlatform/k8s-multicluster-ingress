# HTTPS multicluster ingress

Google Clould Load Balancer (GCLB) supports multiple HTTPS modes:
* [Frontend
  HTTPS](https://github.com/kubernetes/ingress-gce/blob/master/README.md#frontend-https):
  In this mode, all traffic from clients to the load balancer is over HTTPS.
* [Backend
  HTTPS](https://github.com/kubernetes/ingress-gce/blob/master/README.md#backend-https):
  In this mode, all traffic from load balancer to the respective kubernetes
  services is over HTTPS.

Users can choose any combination of these 2 to create an HTTPS multicluster
ingress. For example, they can create an ingress that has only frontend HTTPS and not
backend HTTPS, or they can create an ingress that has both frontend HTTPS and
backend HTTPS.

Multicluster ingress supports the same annotations as a single cluster ingress
to configure an HTTPS ingress.

## Frontend HTTPS

To configure frontend HTTPS, users need to specify an SSL cert that the load
balancer should use. They can specify it as a kubernetes secret or as a GCP SSL
Cert.

Refer to documentation [here](https://github.com/kubernetes/ingress-gce/blob/master/README.md#frontend-https) for more details.

### Caveat

When using a kubernetes secret to specify the SSL cert, kubemci can pick any
cluster to read the desired secret. Hence the user is required to manage the
secret in all clusters and ensure that they are in sync.

In addition, updating the secret is not yet supported.

https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues/141 has
more details and instructions on how to use a GCP SSL cert instead.

## Backend HTTPS

To configure backend HTTPS, users need to annotate their service to specify
which ports support HTTPS.

Refer to documentation
[here](https://github.com/kubernetes/ingress-gce/blob/master/README.md#backend-https)
for more details.

## Blocking HTTP

Users can block HTTP traffic by adding an annotation to their Ingress resource.

Refer to documentation
[here](https://github.com/kubernetes/ingress-gce/blob/master/README.md#blocking-http)
for more details.
