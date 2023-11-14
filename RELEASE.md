# How to publish a new version to the Operator Hub

This operator is published via the OpenShift Community Catalog. See here: https://github.com/redhat-openshift-ecosystem/community-operators-prod/tree/main/operators/cluster-relocation-operator

In order to publish a new version to the catalog, follow these steps:

1. Create a new [release](https://github.com/RHsyseng/cluster-relocation-operator/releases) in this repo (for example: v0.9.10). The release does not need to have any artifacts
2. Creating the release should publish new images to [quay.io](https://quay.io/repository/rhsysdeseng/operators/cluster-relocation-operator?tab=tags)
3. Creating the release will trigger the ["Build operator"](https://github.com/RHsyseng/cluster-relocation-operator/actions/workflows/operator.yaml) GitHub Action. If you look at the action that is triggered, it will generate an artifact named "bundle".
4. Download this "bundle" folder and then open a PR against https://github.com/redhat-openshift-ecosystem/community-operators-prod. Rename the bundle folder to the version number (for example: 0.9.10) and place it here: https://github.com/redhat-openshift-ecosystem/community-operators-prod/tree/main/operators/cluster-relocation-operator. You can look at previous releases for an example of the expected folder structure. Commits to the community-operators-prod repo must be signed (`git commit -s`)
5. PRs to the community-operators-prod repo are fully automated. Assuming you did everything correctly, after the CI is successful, a robot will merge the PR, and after a few minutes, the new version will be available on the Community Catalog.
