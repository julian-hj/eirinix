# Eirinix
[![godoc](https://godoc.org/github.com/SUSE/eirinix?status.svg)](https://godoc.org/github.com/SUSE/eirinix)
[![Build Status](https://travis-ci.org/SUSE/eirinix.svg?branch=master)](https://travis-ci.org/SUSE/eirinix)
[![go report card](https://goreportcard.com/badge/github.com/SUSE/eirinix)](https://goreportcard.com/report/github.com/SUSE/eirinix)
[![codecov](https://codecov.io/gh/SUSE/eirinix/branch/master/graph/badge.svg)](https://codecov.io/gh/SUSE/eirinix)

Extensions Library for Cloud Foundry Eirini

## How to use


### Install
    go get -u github.com/SUSE/eirinix

### Write your extension

An Eirini extension is a structure which satisfies the ```eirinix.Extension``` interface.

The interface is defined as follows:

```golang
type Extension interface {
	Handle(context.Context, Manager, *corev1.Pod, types.Request) types.Response
}
```

For example, a dummy extension (which does nothing) would be:

```golang

type MyExtension struct {}

func (e *MyExtension) Handle(context.Context, eirinix.Manager, *corev1.Pod, types.Request) types.Response {
	return types.Response{}
}
```


### Start the extension with eirinix

```golang

import "github.com/SUSE/eirinix"

func main() {
    x := eirinix.NewManager(
            eirinix.ManagerOptions{
                Namespace:  "kubernetes-namespace",
                Host:       "listening.eirini-x.org",
                Port:       8889,
                // KubeConfig can be ommitted for in-cluster connections
                KubeConfig: kubeConfig,
        })

    x.AddExtension(&MyExtension{})
    log.Fatal(x.Start())
}

```

### Issues

Kubernetes fails to contact the `eirini-extensions` mutating webhook if they are set in `mandatory mode`. This will make any pod fail that is meant to be patched by eirini. An indication that this is happening is that any app being publishesd using `cf push` is creating timeouts.
When running ```kubectl get events -n eirini``` lines of log containing

`Job Warning FailedCreate job-controller Error creating: Internal error occured`

are shown.


#### Fix for a running cluster

In order to trigger re-generation of the mutating webhook certificate, we have to delete the secrets and the associated mutating webhook:

- run `kubectl delete secret eirini-extensions-webhook-server-cert -n eirini`
- run `kubectl delete mutatingwebhookconfiguration eirini-x-mutating-hook-default`
- connect to the `eirini-0` pod (`kubectl exec -it eirini-0 -n eirini /bin/bash`)
- run `monit restart eirini-extensions`


#### Fix on redeploy on an existing k8s (which had a scf deployed before):
- In case of multiple re-deployments on the same cluster it can happen that old secrets are still present on the cluster. The eirinix library then tries to reuse those, resulting in a failed connection since the service will have a different IP-address.

- Before redeploying run `kubectl get secrets -n eirini`, if there is an `eirini-x-setupcertificate` (the name may vary depending on the operator fingerprint set on the extension, see [https://godoc.org/github.com/SUSE/eirinix#ManagerOptions
](https://godoc.org/github.com/SUSE/eirinix#ManagerOptions
) for details) present, delete it using `kubectl delete secret eirini-x-setupcertificate -n eirini`

  We need also to remove the older mutatingwebhook:
    ```
    $> kubectl get mutatingwebhookconfiguration
    NAME                                     CREATED AT
    eirini-x-mutating-hook-default   2019-06-10T08:55:30Z

    $> kubectl delete mutatingwebhookconfiguration eirini-x-mutating-hook-default
    ```
