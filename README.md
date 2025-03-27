# node-exporter-operator
A Kubernetes operator that simplifies the deployment and management of Prometheus monitoring components including Node Exporter, Blackbox Exporter, and Kube State Metrics.

## Description
This operator provides a unified way to deploy and manage three essential Prometheus monitoring components in your Kubernetes cluster:

1. **Node Exporter**: Collects hardware and OS metrics from cluster nodes, such as CPU usage, memory, disk I/O, and network statistics.
2. **Blackbox Exporter**: Probes endpoints over HTTP, HTTPS, DNS, TCP and ICMP, enabling monitoring of endpoints and services both inside and outside your cluster.
3. **Kube State Metrics**: Generates metrics about the state of various Kubernetes objects, including deployments, pods, and nodes.

Each component can be enabled or disabled independently through the NodeExporter custom resource. The operator handles the complete lifecycle of these components, including:
- Creating necessary ServiceAccounts and RBAC permissions
- Managing DaemonSets for each component
- Configuring the Blackbox Exporter through a ConfigMap
- Setting appropriate resource limits and security contexts
- Cleaning up resources when components are disabled

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/node-exporter-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/node-exporter-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/node-exporter-operator:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/node-exporter-operator/<tag or branch>/dist/install.yaml
```

## Contributing
We welcome contributions to the node-exporter-operator project! Here's how you can contribute:

1. **Bug Reports**: Open an issue describing the bug, including steps to reproduce and expected behavior.

2. **Feature Requests**: Submit an issue describing the new feature and its potential benefits.

3. **Code Contributions**:
   - Fork the repository
   - Create a new branch for your feature/fix
   - Write tests for new functionality
   - Ensure all tests pass by running `make test`
   - Submit a pull request with a clear description of changes

4. **Documentation**: Help improve documentation by fixing typos, adding examples, or clarifying instructions.

Please ensure your contributions adhere to our coding standards and include appropriate tests and documentation.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
