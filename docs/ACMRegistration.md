# ACM Registration

The operator has the ability to register a cluster to ACM. In order to do this, you fill out the optional `acmRegistration` field in the spec:
```
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
```

The `acmSecret` Secret requires a `token` field under the `data` section of the Secret. This is a Service Account token from the ACM cluster. Optionally, a `ca.crt` data field can also be provided, in order to communicate with an ACM cluster that uses a self-signed certificate for its API.

## Generating the acmSecret

Run these commands on your ACM cluster:
```
oc create sa -n multicluster-engine acm-registration-sa
oc adm policy add-cluster-role-to-user open-cluster-management:managedclusterset:admin:default -n multicluster-engine -z acm-registration-sa

TOKEN=$(oc create token -n multicluster-engine acm-registration-sa --duration=720h | base64 -w 0)
BASE_DOMAIN=$(oc get dns cluster -o jsonpath='{.spec.baseDomain}')
SERVER_CERT=$(echo | timeout 5 openssl s_client -showcerts -connect "api.${BASE_DOMAIN}:6443" 2>/dev/null | openssl x509 | base64 -w 0)

cat << EOF > /tmp/acm-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: acm-secret
  namespace: openshift-config
data:
  token: ${TOKEN}
  ca.crt: ${SERVER_CERT}
EOF
```

Run this command on your target cluster:
```
oc apply -f /tmp/acm-secret.yaml
```

Now your target cluster has a Secret than will allow it to authenticate to the ACM cluster and register itself. Once the registration succeeds, the secret is deleted from the target cluster.
