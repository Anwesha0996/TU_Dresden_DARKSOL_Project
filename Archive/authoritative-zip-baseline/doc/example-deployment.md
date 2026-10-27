# Example Deployment Process

## Overview

This document outlines the process of deploying an application using `helm-dtn`. It covers the necessary steps for preparing the local and remote environment, configuring `helm-dtn` and how to manage an application. For more in-depth information about how `helm-dtn` works, please check the documentation on `helm-dtn`'s [Mode of Operation](mode-of-operation.md).

### Preparing the remote environment

To use `helm-dtn` to manage applications in a remote or airgapped Kubernetes cluster, three components need to be deployed to the cluster:
- a Bundle Protocol Agent (BPA)
- the `helm-dtn` Application Lifecycle Management Engine (ALME)
- a registry

We recommend using [µD3TN](https://gitlab.com/d3tn/ud3tn) as the Bundle Protocol Agent, as it was also used during development and testing and supports the Application Agent Protocol in version 2.0 (AAP2), which `helm-dtn` uses for connecting to a BPA.

An image for running µD3TN can be found in the [official µD3TN Gitlab Docker Registry](https://gitlab.com/d3tn/ud3tn-docker-images/container_registry), whereas images for the `helm-dtn` ALME are available via the project's [Github Container Registry](https://github.com/darksol-cloud/helm-dtn/pkgs/container/helm-install).

To deploy µD3TN and the `helm-dtn` ALME, you will need to create two Kubernetes manifests. You may use the following examples as a baseline:

<details>

<summary><b>Manifest for µD3TN</b></summary>

The following is an example configuration for µD3TN. It contains definitions for a deployment and a service opening a NodePort to make it reachable from outside the cluster. It is mostly ready for deployment, you just need to replace `<your-image-here>` with a reference to the µD3TN image you wish to use. You will also need to configure an AAP2 secret, for which we also provide an example configuration further down below.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ud3tn
spec:
  replicas: 1
  selector:
    matchLabels:
      app: ud3tn
  template:
    metadata:
      labels:
        app: ud3tn
    spec:
      containers:
      - name: ud3tn
        image: <your-image-here>
        ports:
        - containerPort: 4242
        - containerPort: 4556
        args: ["--node-id", "dtn://ud3tn.dtn/", "--aap2-host", "0.0.0.0", "--aap2-port", "4242", "--bdm-secret-var", "AAP2_SECRET", "--log-level", "4"]
        env:
        - name: AAP2_SECRET
          valueFrom:
            secretKeyRef:
              name: aap2-secret
              key: token
---
apiVersion: v1
kind: Service
metadata:
  name: ud3tn-service
spec:
  selector:
    app: ud3tn
  ports:
  - name: "aap"
    port: 4242
    targetPort: 4242
  - name: "tcp-external"
    port: 4556
    targetPort: 4556
    nodePort: 31313
  type: NodePort
```

**Important configuration options:**
- `--node-id`: the node ID which is used by the started µD3TN instance for its administrative endpoint. This is used for deriving the endpoint identifiers of integrated agents as well as for applications connected through AAP and AA2. So, for example, if you registered an application with the agent name `test-app`, the full Endpoint Identifier (EID) in this example would be `dtn://ud3tn.dtn/test-app/`
- `--aap2-port`: specifies the TCP port number to which the AAP 2.0 service should be bound. If you change this, you will also need to update the port definitions in the `ud3tn-service`.

For a full overview of available options and their implications please refer to the [µD3TN manpage](https://d3tn.gitlab.io/ud3tn/references/manpage/).

**AAP 2 Secret**

In order to use µD3TN, a secret containing the AAP2 Token applications authenticate against µD3TN with has to be created. The following manifest creates such a secret and has to be applied **before** µD3TN.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: aap2-secret
type: Opaque
data:
  token: <your-AAP2-token-in-base64-here> # e.g. "dGVzdA==" for "test"
```

</details>

<details>

<summary><b>Manifest for the helm-dtn ALME</b></summary>

The following is an example configuration for the `helm-dtn` ALME. It is a bit more complex than the µD3TN definition as `helm-dtn` requires privileges for creating, upgrading and deleting resources. For this reason, a `ServiceAccount` named `helm-worker-sa` and a `Role` consolidating these required authorizations is created. The `ServiceAccount` and the `Role` are then connected via a `RoleBinding`, which allows us to add the `ServiceAccount` to the actual `Deployment`. With this, the `helm-dtn` ALME is able to get, list, watch, create, update, patch and delete deployments, services, pods, configmaps, secrets, replicasets, statfulset, jobs and ingresses. If you already know that some of these authorizations are not needed, please update the `helm-worker-role` `Role` to the minimal set of authorizations.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: helm-worker-sa
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: helm-worker-role
rules:
  - apiGroups: ["", "apps", "batch", "networking.k8s.io"]
    resources: ["deployments", "services", "pods", "configmaps", "secrets", "replicasets", "statefulsets", "jobs", "ingresses"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: helm-worker-binding
subjects:
- kind: ServiceAccount
  name: helm-worker-sa
roleRef:
  kind: Role
  name: helm-worker-role
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: helm-install-deployment
  labels:
    app: helm-install
spec:
  replicas: 1
  selector:
    matchLabels:
      app: helm-install
  template:
    metadata:
      labels:
        app: helm-install
    spec:
      serviceAccountName: helm-worker-sa
      volumes:
      - name: secret-volume
        secret:
          secretName: aap2-secret
      containers:
      - name: helm-install
        image: <your-image-here>
        env:
        - name: DTN_SERVICE_ADRESS
          value: "ud3tn-service.default.svc.cluster.local"
        - name: DTN_SERVICE_PORT
          value: "4242"
        - name: REGISTRY_ADDRESS
          value: <your-registry-address-or-ip-and-port-here>
        volumeMounts:
        - name: secret-volume
          readOnly: true
          mountPath: "/etc/secret-volume"
```

This is mostly ready for deployment, except for two fields:
- you need to replace `<your-image-here>` with a reference to your image of the `helm-dtn` ALME
- you need to replace `<your-registry-address-or-ip-and-port-here>` with the address of the local registry running in your cluster
</details>


To apply the manifests, copy the YAML manifests into a file (e.g. `ud3tn.yaml` for the µD3TN configuration and `helm-dtn.yaml` for the `helm-dtn` config). You may then use `kubectl` to deploy these components as follows:
- µD3TN AAP2 Secret: `kubectl apply -f aap2secret.yaml`
- µD3TN: `kubectl apply -f ud3tn.yaml`
- helm-dtn: `kubectl apply -f helm-dtn.yaml`


### Preparing your local environment

Following the setup of the remote cluster, you will also need to set up your local environment.

#### Setting up µD3TN
To interact with a remote/airgapped cluster, a running instance of a Bundle Protocol implementation with support for the AAP2 is required. Please follow the [guide on building and running µD3TN](https://d3tn.gitlab.io/ud3tn/usage/build-and-run/) to start a µD3TN instance on you local machine. You can use the default settings.

##### Connecting to the remote cluster

Once you have a µD3TN instance running on your system, you can connect to the remote cluster. For this, you need to configure a link. The process for two local µD3TN instances is described in the [µD3TN Quick Start Guide](https://d3tn.gitlab.io/ud3tn/posix_quick_start_guide/#configure-d3tn-instances), and is very similar to what you need to do. If you used the configuration for the µD3TN Deployment presented earlier, you need to configure a TCP link. Using the command shown in the µD3TN Quick Start Guide:
```
# Enable Python virtual environment first!!
aap2-config --socket <ud3tn-aap2-socket> \
    --schedule 1 3600 1000000 \
    <node-id-remote-ud3tn> tcpclv3:<cluster-ip>:31313
```

Here, `<ud3tn-app2-socket>` corresponds to the socket over which your local instance is reachable. If you started µD3TN with the default configuration, replace the placeholder with `ud3tn.aap2.socket`. Next, the `--schedule` flag tells µD3TN when the link starts, how long it will be active and what the expected transmission rate is. In this case, it will start 1s after the command was issued, will last for 3600s (1h) and have an expected transmission rate of 1,000,000 bytes per second. `<node-id-remote-ud3tn>` is a placeholder for the Node ID of your remote instance. If you did not change the config, this will be `dtn://ud3tn.dtn/`. Lastly, you need to specify over which CLA the node is reachable. We configured the NodePort of the remote cluster to forward to `4556`, which is the default port for the TCP Convergence Layer (TCPCL). We therefore also use the TCPCL in this command (-> `tcpclv3`), you just need to substitute `cluster-ip` with the IP of the node µD3TN is running on.

Once you issue this command, you should see your local node trying (and hopefully succeeding!) to establish a connection to the remote node. When the connection has been established, you may continue with setting up `helm-dtn`.

#### Setting up `helm-dtn`
Luckily, installing `helm-dtn` is straightforward, as you can use the standard `helm plugin install` command:
```
helm plugin install https://github.com/darksol-cloud/helm-dtn
```
You can then use the plugin by calling `helm dtn <command>`.

Alternatively, the binary provided in this repository can be run on its own without installing it anywhere:
`./helm-dtn <command>`.

To generate a configuration file for `helm-dtn` run `helm dtn configure` (or `./helm-dtn configure`). This will prompt you to provide the following information:
- the EID of the cluster charts should be deployed to (`dtn://ud3tn.dtn/` if you didn't change the µD3TN manifest provided above)
- the address of the local registry running in the remote cluster (this would be the same one you added to the helm-dtn manifest)
- whether you want to connect to µD3TN using a socket or TCP (if you start µD3TN with the default settings you need to choose `socket` here)
- the path to the UNIX domain socket (default: `<directory-you-ran-ud3tn-from>/ud3tn.aap2.socket`) if you chose socket or the address of the AAP2 service (default: `localhost:4242`) if you chose TCP

This will create a configuration file at `.local/share/helm-dtn/config.yaml` which will look similar to this example:
```yaml
destinationEid: dtn://ud3tn.dtn/
registry: 192.168.42.2:30500
socket: ""
tcp: "127.0.0.1:4242"
```
This configuration file is then automatically used by `helm-dtn` for future command invocations. If your cluster configuration changes, you need to run `helm dtn configure` again, or update the config file directly.

It is also possible to use `helm-dtn` directly, without creating a configuration file by supplying command-line flags instead. Please run `helm dtn -h` to get an overview of the available commands, and `helm dtn <command> -h` to get the flags available for the chosen command.



### Deploying an application

Once your `helm-dtn` instance is configured, you can use it to deploy an application. The process is very simple, all you need is a Helm chart of the application you want to deploy. You can then invoke `helm-dtn` as follows:
```
helm dtn install <path-to-helm-chart>
```

This will automatically use the configuration created in the previous step to fetch dependencies, update the chart to reference local resources and send it to the remote cluster.

The full process of how `helm-dtn` resolves, downloads and packages dependencies is outlined in the [documentation of `helm-dtn`'s mode of operation](mode-of-operation.md).

### Upgrading an application

Upgrading an application is done in a very similar way. You just need the new Helm Chart with updated image references and run:

```
helm dtn upgrade <path-to-chart>
```

Please ensure that the name of the chart is not different to the previous version, as this is the identifier the `helm-dtn` ALME uses for upgrading the correct release.

### Deleting an application

For deleting an application you need to provide the release name, under which it has been deployed. You can either find it using `helm list` in the remote cluster, or just by checking the `Chart.yaml` of the application. It should look something like this:
```yaml
apiVersion: v2
name: example-app
description: An example Helm chart for the helm-dtn docs
version: 0.1.0
appVersion: "1.0.0"
```

In this case the name of the app would be `example-app`. The `helm-dtn` ALME takes this name and appends `-release`, making the release name of this example application `example-app-release`.

With the release name you can then invoke the following command:

```
helm dtn uninstall <release-name>
```

In our example:

```
helm dtn uninstall example-app-release
```
