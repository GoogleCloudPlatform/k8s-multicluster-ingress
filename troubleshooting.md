# Troubleshooting

## Debugging
To provide valuable feedback you can run kubemci with glog flags (like
`--logtostderr=true --stderrthreshold=INFO --v=10`) to provide (very) verbose
output. This can contain relevant hints as to why kubemci might fail.

Example:

```bash
kubemci create my-mci --logtostderr=true --stderrthreshold=INFO --v=10 --ingress=example.yaml --gcp-project=myproj --kubeconf=config.yaml
```

## Error: “cannot fetch token: 400 Bad Request”

```
$ kubemci create ...
E1019 11:54:23.663565   33382 gce.go:745] error fetching initial token: oauth2: cannot fetch token: 400 Bad Request
Response: {
  "error" : "invalid_grant",
  "stack_trace" : "com.google.security.lso.protocol.oauth2.common.OAuth2ErrorException: invalid_grant\n\tat com.google.security.lso.protocol.oauth2.com

... ✄ ✄ snip ✄ ✄ ...

Error in creating load balancer: error in creating cloud interface: timed out waiting for the condition
```

Most likely you are not logged in using gcloud. Please run the following command to login:
```
gcloud auth application-default login
```

## Error: “Invalid value for field 'resource.name'”

```
$ kubemci create ... myLbName

... ✄ ✄ snip ✄ ✄ ...

Error googleapi: Error 400: Invalid value for field 'resource.name': 'mci1-hc-32013--myLbName'. Must be a match of regex '(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)'
```

In this case, we tried to use a CamelCase name `myLbName` for the name of the load-balancer when calling `kubemci create … myLbName`. In GCP, upper-case letters are invalid (as well as underscores); so instead we use dashes (e.g.; `my-lb-name`).

## Error: “IP address is in-use”

```
$ kubemci create ...

... ✄ ✄ snip ✄ ✄ ...

googleapi: Error 400: Invalid value for field 'resource.IPAddress': '35.190.48.245'. Specified IP address is in-use and would result in a conflict., invalid
```

The IP address is in use. This can happen if 2 load balancers are sharing the same IP address.

You can find out the resource using it by running `gcloud compute addresses describe <ip-address-name>`.
`ip-address-name` being used by the ingress can be found from value of
`kubernetes.io/ingress.global-static-ip-name` annotation.

## Known issues

You can view the list of all known issues/limitations at https://github.com/GoogleCloudPlatform/k8s-multicluster-ingress/issues
