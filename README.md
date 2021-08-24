Welcome to the open source release version 1.1.1 of ONF's SD-RAN project. Please note that this is a read only release of the source code. We will not be accepting pull requests in these repos, and the source code that is contained here cannot be used to build functional binaries because it refers back to a number of private repositories. But executables - docker containers referencing the released code - are available on Docker Hub and referenced [here](https://wiki.opennetworking.org/display/COM/SD-RAN+1.1+Release).  SD-RAN is currently a member-only project, and ONF membership is required to access the most current release, the master branch, and to do development with SD-RAN.

# onos-kpimon
The xApplication for ONOS SD-RAN (µONOS Architecture) to monitor KPI

## Overview
The `onos-kpimon` is the xApplication running over ONOS SD-RAN to monitor the KPI.
As of now, it monitors the number of active UEs in the Radio Access Network (RAN) connected to the ONOS SD-RAN.
Since ONOS SD-RAN has multiple micro-services running on the Kubernetes platform, `onos-kpimon` can run on the Kubernetes along with other ONOS SD-RAN micro-services.
In order to deploy `onos-kpimon`, a Helm chart is necessary, which is in the [`sdran-helm-charts`](https://github.com/onosproject/sdran-helm-charts) repository.
Note that this application should be running together with the other SD-RAN micro-services (e.g., `Atomix`, `onos-e2t`, `onos-e2sub`, and `onos-sdran-cli`).
Easily, `sd-ran` umbrella chart can be used to deploy all essential micro-services and `onos-kpimon`. 

## Interaction with the other ONOS SD-RAN micro-services
To begin with, `onos-kpimon` sends a subscription message to [`onos-e2sub`](https://github.com/onosproject/onos-e2sub) to receives E2 indication messages through [`onos-ric-sdk-go` library](https://github.com/onosproject/onos-ric-sdk-go).
When the subscription is finished successfully, `onos-kpimon` application starts to get E2 indication messages from E2 node, such as `CU-CP`, through [`onos-e2t`](https://github.com/onosproject/onos-e2t).
Then, `onos-kpimon` decodes each indication message by using E2 KPM service model which is defined in [`onos-e2-sm` plugin](https://github.com/onosproject/onos-e2-sm).
The monitoring result can be shown with the CLI through [`onos-sdran-cli`](https://github.com/onosproject/onos-cli).
`onos-kpimon` sends the monitoring result to the `onos-sdran-cli` through gRPC protocol defined in [`onos-api`](https://github.com/onosproject/onos-api) repository.

## Command Line Interface
### Show the number of UEs
Go to the CLI micro-service pod, and command below:
```bash
$ sdran kpimon list numues
Key[PLMNID, nodeID]                       num(Active UEs)
{eNB-CU-Eurecom-LTEBox [0 2 16] 57344}   1
```
