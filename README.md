# cluster-relocation-operator
![CI Status](https://github.com/RHsyseng/cluster-relocation-operator/actions/workflows/operator.yaml/badge.svg)
![Tests](https://github.com/RHsyseng/cluster-relocation-operator/actions/workflows/tests.yaml/badge.svg)

:warning: **This is a community project, it is not supported by Red Hat in any way.** :warning:

## Description
This operator can assist in reconfiguring a cluster once it has been moved to a new location. It performs the following steps:

* Update the API and Ingress domain aliases using a generated certificate (signed by loadbalancer-serving-signer), or using a user provided certificate.
* Update the internal DNS records for the API and Ingress (SNO only).
* (Optional) Update the cluster-wide pull secret.
* (Optional) Add new SSH keys for the 'core' user.
* (Optional) Add new CatalogSources.
* (Optional) Add new ImageContentSoucePolicy/ImageDigestMirrorSets for mirroring.
* (Optional) Add new trusted CA for a mirror registry.
* (Optional) Register the cluster to ACM.

The cluster needs to be able to resolve the API and ingress (*.apps) addresses for the new domain. On SNO, you can set the `addInternalDNSEntries` key to `true` in the CR spec in order to add internal DNS entries via dnsmasq. Enabling this option will cause the node to reboot, because a MachineConfig is applied.

## Getting Started
You’ll need an OpenShift cluster to run against. The cluster must be v4.12 or higher.

### Installing on the cluster
```
cat << EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: "cluster-relocation-operator"
  namespace: "openshift-operators"
spec:
  channel: "stable"
  installPlanApproval: "Automatic"
  source: "community-operators"
  sourceNamespace: "openshift-marketplace"
  name: "cluster-relocation-operator"
EOF
```
Once the operator is installed:
```
cat << EOF | oc apply -f -
apiVersion: rhsyseng.github.io/v1beta1
kind: ClusterRelocation
metadata:
  name: cluster
spec:
  domain: sample.new.domain.com
  acmRegistration:
    url: https://api.hub.example.com:6443
    clusterName: sample
    acmSecret:
      name: acm-secret
      namespace: openshift-config
    klusterletAddonConfig:
      policyController:
        enabled: true
      applicationManager:
        enabled: true
      certPolicyController:
        enabled: true
      iamPolicyController:
        enabled: true
      searchCollector:
        enabled: true
  catalogSources:
    - name: new-catalog-source
      image: <mirror_url>:<mirror_port>/redhat/redhat-operator-index:v4.12
  imageDigestMirrors:
    - mirrors:
        - <mirror_url>:<mirror_port>/lvms4
      source: registry.redhat.io/lvms4
  pullSecretRef:
    name: my-new-pull-secret
    namespace: my-namespace
  registryCert:
    registryHostname: <mirror_url>
    registryPort: 8443
    certificate: <new_registry_cert>
  sshKeys:
    - <new_ssh_key>
EOF
```
### Deleting the CR
When you delete the ClusterRelocation CR, everything will be reverted back to its original state.

Optionally, you may add the `self-destruct: "true"` annotation when you create the CR:
```
apiVersion: rhsyseng.github.io/v1beta1
kind: ClusterRelocation
metadata:
  name: cluster
  annotations:
    self-destruct: "true"
```

This annotation will cause the operator to delete itself (the CR and the Subscription) once the reconciliation has completed, while allowing the cluster to retain all of the new configuration.
This is useful if you want to remove any operator related overhead on the cluster after the relocation configuration has been applied.

## Contributing
This is a community project, feel free to open a PR and help out!

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/),
which provide a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

