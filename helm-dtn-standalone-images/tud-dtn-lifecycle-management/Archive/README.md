# TU Dresden – DTN Lifecycle Management Project

## Overview

This repository contains practical experiments and documentation related to **Delay/Disruption Tolerant Networking (DTN)**, **Talos Linux**, **Kubernetes**, and **Helm-based application deployment** for disconnected or space-network environments.

The project focuses on understanding how Kubernetes clusters and containerized applications can be managed in environments where continuous internet connectivity is not guaranteed.

The work was completed as part of research and learning activities related to distributed cloud systems and DTN-based infrastructure management.

---

# Project Goals

The main goals of this repository are:

* Understand Talos Linux and Kubernetes cluster deployment
* Explore Helm-based application deployment
* Simulate disconnected / air-gapped deployment workflows
* Study DTN-oriented lifecycle management concepts
* Prepare a foundation for Kubernetes management over DTN networks

---

# Repository Structure

```text
.
├── rust-image-transfer/
├── project-01-talos-k8s/
├── project-02-helm-deployment/
├── project-03-airgapped-helm/
├── Earth to Space and Proj 6 & 7 .drawio (1).pdf
├── HelmDtn_Anwesha Ghosh.drawio
├── Task 1_helmdtn text.docx
├── Task 2  & 3.docx
└── Readme.md.docx
```

`rust-image-transfer/` contains the superseded Rust image-transfer prototype,
its demo environments, and supporting documentation. The active Go-based
standalone image-transfer implementation is maintained at the repository root.

---

# Task 1 – Helm DTN Deployment

Folder:

```text
Task 1_helmdtn text.docx
```

## Description

Task 1 focuses on understanding how Helm charts and Kubernetes applications can be transferred and deployed in disconnected or DTN-based environments.

The task studies the concept of packaging applications, dependencies, and container images into transferable bundles.

## Main Objectives

* Understand Helm chart structure
* Learn Helm packaging workflow
* Explore DTN-based application transfer concepts
* Simulate disconnected deployment scenarios
* Study Earth-to-space deployment workflow

## Workflow

### Earth Side

* Create Helm chart
* Package application
* Store dependencies
* Prepare transferable bundle

### Space Side

* Receive deployment bundle
* Deploy application in Kubernetes
* Verify services and pods

## Tools Used

* Helm
* Kubernetes
* DTN concepts
* kubectl

---

# Task 2 & 3 – Talos and Kubernetes Lifecycle Management

Folder:

```text
Task 2  & 3.docx
```

## Description

Task 2 and Task 3 focus on Talos Linux, Kubernetes lifecycle management, and air-gapped cluster operation.

The objective is to understand how Kubernetes clusters can be managed in disconnected environments using Talos Linux and related tools.

## Main Objectives

* Install and configure Talos Linux
* Create Kubernetes cluster
* Manage nodes using talosctl
* Deploy applications in Kubernetes
* Explore lifecycle management concepts
* Understand offline cluster operation

## Main Activities

* Kubernetes cluster setup
* Node verification
* Pod deployment
* Service exposure
* Helm-based deployment
* Air-gapped deployment simulation

## Tools Used

* Talos Linux
* Kubernetes
* kubectl
* talosctl
* Helm

---

# Project 01 – Talos Kubernetes Setup

Folder:

```text
project-01-talos-k8s/
```

## Description

This project demonstrates the setup of a Kubernetes cluster using **Talos Linux**.

Talos is a minimal and API-driven operating system designed specifically for Kubernetes.

## Main Tasks

* Install Talos Linux
* Configure Kubernetes cluster
* Connect using `talosctl`
* Verify Kubernetes nodes
* Deploy test applications

## Tools Used

* Talos Linux
* talosctl
* Kubernetes
* kubectl

---

# Project 02 – Helm Deployment

Folder:

```text
project-02-helm-deployment/
```

## Description

This project demonstrates how Helm can be used for Kubernetes application deployment.

Helm simplifies Kubernetes deployment using reusable charts.

## Main Tasks

* Install Helm
* Add Helm repositories
* Deploy applications using Helm charts
* Manage releases
* Verify running services and pods

## Tools Used

* Helm
* Kubernetes
* kubectl

---

# Project 03 – Air-Gapped Helm Deployment

Folder:

```text
project-03-airgapped-helm/
```

## Description

This project simulates an **air-gapped environment** where internet connectivity is unavailable.

The workflow demonstrates how Helm charts and container images can be packaged on the Earth side and transferred to another isolated environment.

This concept is important for:

* Space communication systems
* DTN-based deployments
* Remote environments
* Offline Kubernetes management

## Workflow

### Earth Side

* Download Helm chart
* Package chart
* Store required dependencies
* Prepare deployment bundle

### Space Side / Offline Side

* Transfer packaged files
* Install application without internet
* Verify deployment inside Kubernetes

## Tools Used

* Helm
* Kubernetes
* Talos Linux
* Container Registry

---

# DTN Lifecycle Management Concept

The broader research idea behind this work is:

* Managing Kubernetes clusters in disconnected networks
* Supporting lifecycle operations over DTN
* Deploying applications across Earth-to-space communication environments
* Investigating air-gapped cluster management

The architecture includes:

* Earth-side systems
* DTN transport layer
* Service virtual machines
* Kubernetes execution environments
* Helm-based application deployment

---

# Technologies Used

| Technology  | Purpose                              |
| ----------- | ------------------------------------ |
| Talos Linux | Kubernetes operating system          |
| Kubernetes  | Container orchestration              |
| Helm        | Kubernetes package management        |
| DTN         | Delay/Disruption Tolerant Networking |
| kubectl     | Kubernetes CLI                       |
| talosctl    | Talos management CLI                 |

---

# Learning Outcomes

Through these projects, the following concepts were explored:

* Kubernetes cluster deployment
* Talos Linux management
* Helm packaging and deployment
* Air-gapped deployment workflows
* DTN-based infrastructure ideas
* Offline application lifecycle management

---

# Author

**Anwesha Ghosh**
Master’s Student – Information and Communications Engineering
Alpen-Adria-Universität Klagenfurt (AAU)
Erasmus Student at TU Dresden

---

# Notes

This repository is intended for educational and research purposes.

Some files contain experimental configurations, project notes, and workflow demonstrations related to Kubernetes lifecycle management in disconnected environments.
