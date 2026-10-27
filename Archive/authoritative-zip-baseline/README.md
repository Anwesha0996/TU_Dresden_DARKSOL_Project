# helm-dtn

This Helm plugin facilitates transferring charts to and managing their lifecycle in remote or airgapped Kubernetes clusters.

## Introduction
When installing or updating software in systems without access to the Internet, the status quo is to manually transfer the required binaries and/or dependencies e.g. via an USB stick. While this works in regular deployments, there may be environments where such a manual transfer is not possible or requires  inproportionate amounts of resources (e.g. clusters in space, for remote sensing, or in underwater networks).

This plugin aims to address this issue by providing a way to install, upgrade and uninstall applications in such airgapped clusters through the use of Delay- and Disruption-tolerant Networking (DTN).

It assists in fetching required dependencies, bundling them with the chart and deploying everything over a one-way DTN link to a Kubernetes cluster.

## Architecture
The following is a high-level overview of the components that make up `helm-dtn` and how they interact. In this case, the example of running `helm dtn install` is used, but the process is identical for the `ugrade` command.

![helm-dtn-overview](/doc/img/helmdtn.png)

`helm-dtn` consists of four sub-components:
- `helmdd`: a component for detecting dependencies (such as e.g. images) in Helm charts
- `helmdl`: a component for downloading the dependencies detected by `helmdd`
- `helmdp`: a component for updating the Helm chart to point to the local resources and pack everything in a .tar.gz archive
- `helmdtnt`: a component for interfacing with a Bundle Protocol Agent, which forwards the previously created archive to a remote cluster

These components work in tandem to automate deployment, upgrades and deletion of resources in a remote repository and are orchestrated by the commands defined in [/cmd](/cmd/). When such a command is invoked, the components are executed in sequence, starting with `helmdd`, which will analyze the provided Helm chart and note all discovered dependencies in a YAML file. This is extensible, meaning it is straightforward to implement checks for dependencies other than images. An example of how to integrate checks for Git repositories is provided in the component's [documentation](/pkg/helmdd/README.md).

The resulting YAML file is then used by `helmdl` to download the required dependencies, before `helmdp` is invoked and updates the Helm chart to reference the newly downloaded resources instead of the previously defined remote versions. It also creates a `.tar.gz` archive, which contains the newly updated Helm chart, as well as all downloaded dependencies.

This archive is then forwarded to a Bundle Protocol Agent (e.g. µD3TN), which forwards it to the remote cluster, in which the Application Lifecycle Management counterpart of `helm-dtn` is running.


A full explanation of how `helm-dtn` works including a focus on data flows and application lifecycle management in the remote cluster is provided in the [Mode of Operation](/doc/mode-of-operation.md) documentation.

## Installation
This software can be installed using `helm plugin install`:
```
helm plugin install https://github.com/darksol-cloud/helm-dtn
```

Alternatively, the binary can be run on its own.

## Usage
The general usage is `helm dtn [command] [flags]`. A list of available commands can be found below or by running `helm dtn -h`.

Before using the install, upgrade or uninstall commands, it is recommended to run `helm dtn configure`. This will prompt for all settings required for deploying to a remote cluster and generate a configuration file, which will be automatically used by `helm dtn`. If you do not wish to generate such a configuration file, you can also pass these options as flags to the install, upgrade and uninstall commands.


**Available Commands:**
```
completion  Generate the autocompletion script for the specified shell
configure   Generates a configuration file for helm-dtn commands
help        Help about any command
install     Installs the specified chart and all dependencies.
uninstall   Takes the specified release name and uninstalls the release.
upgrade     Takes the specified chart and upgrades the release to the new version of the chart
```

Use `helm dtn [command] --help` for more information about a command.

## Commands
### `configure`
```
The configure command generates a configuration file used by other helm-dtn commands.

Usage:
  dtn configure [flags]

Flags:
  -h, --help   help for configure
```

### `install`
```
install takes a specified Helm chart and downloads subcharts, required images and other required resources. Once all dependencies are available locally, the chart is bundled and sent over a DTN to an airgapped Kubernetes cluster, where it is subsequently installed.

Usage:
  dtn install <Path to directory containing chart> [flags]

Flags:
  -d, --destination-eid string   EID of the Application Lifecycle Management Engine in the remote cluster.
  -h, --help                     help for install
  -r, --registry string          Name of the local registry running in the remote cluster.
  -s, --socket string            Path to the unix socket created by µD3TN.
  -t, --tcp string               IP and Port µD3TN listens for incoming AAP2 connections on. Example: 127.0.0.1:4244.
```
As mentioned above: if `helm dtn configure` was run beforehand, no flags have to be passed to `helm dtn install`. A path to the chart directory still has to be provided.

The release name of the chart is automatically generated from the chart name by appending `-release` to the chart name. A chart named `demo` would therefore be deployed as `demo-release`. This may change in future versions to allow users to specify their own release names.

### `upgrade`
```
upgrade takes a specified Helm chart and downloads subcharts, required images and other required resources. Once all dependencies are available locally, the chart is bundled and sent over a DTN to an airgapped Kubernetes cluster, where it is subsequently used to upgrade the specified release.

Usage:
  dtn upgrade <Path to directory containing chart> [flags]

Flags:
  -d, --destination-eid string   EID of the Application Lifecycle Management Engine in the remote cluster.
  -h, --help                     help for upgrade
  -r, --registry string          Name of the local registry running in the remote cluster.
  -s, --socket string            Path to the unix socket created by µD3TN.
  -t, --tcp string               IP and Port µD3TN listens for incoming AAP2 connections on. Example: 127.0.0.1:4244.
```
As mentioned above: if `helm dtn configure` was run beforehand, no flags have to be passed to `helm dtn upgrade`. A a path to the chart directory still has to be provided.

### `uninstall`
```
uninstall takes a release name and uninstalls the release. It removes all of the resources associated with the last release of the chart as well as the release history, freeing it up for future use.

Usage:
  dtn uninstall <release-name> [flags]

Flags:
  -d, --destination-eid string   EID of the Application Lifecycle Management Engine in the remote cluster.
  -h, --help                     help for uninstall
  -s, --socket string            Path to the unix socket created by µD3TN.
  -t, --tcp string               IP and Port µD3TN listens for incoming AAP2 connections on. Example: 127.0.0.1:4244.
```
As with the other commands, `helm dtn uninstall` does not require any flags if there is a configuration file.

## Repository Structure
```
.
├── cmd                 implementations of the install/upgrade/delete commands
├── doc                 documentation
├── helm-dtn            helm-dtn binary
├── install.sh          install hook for helm plugin install
├── main.go             main entrypoint, executes the provided command
├── pkg                 implementations of the sub-components
│   ├── aap2_client     temporary addition of the AAP2 Go Client
│   ├── helmdd
│   ├── helmdl
│   ├── helmdp
│   ├── helmdtnt
│   ├── proto           AAP2 Protobuf definitions
│   └── util            utilities for archiving, file handling, etc.
├── plugin.yaml         YAML describing the helm-dtn plugin
└── README.md           You are here
```
