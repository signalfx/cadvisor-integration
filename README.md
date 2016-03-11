[![Build Status](https://travis-ci.org/signalfx/cadvisor-integration.svg?branch=master)](https://travis-ci.org/signalfx/cadvisor-integration) [![Docker Repository on Quay.io](https://quay.io/repository/signalfx/cadvisor-integration/status "Docker Repository on Quay.io")](https://quay.io/repository/signalfx/cadvisor-integration)

# cadvisor-integration
## Overview
This tool will auto discover kubernetes cluster nodes and send container and machine metrics to SignalFx. 
You can deploy it to run in a single pod within a Kubernetes cluster where it will collect data from the cadvisor container resource usage and performance analysis agent (that is integrated into the kubelet binary) using cluster member permissions only. 
## Installation
**Step 1** - Create a deployment configuration based on the following example _cadvisor-signalfx.yaml_ file. **Note:** Change the &lt;API_TOKEN&gt; based on your SignalFx account and the &lt;CLUSTER_NAME&gt; based on what you want to call this kubernetes cluster.

	apiVersion: v1
	kind: ReplicationController
	metadata:
	  name: "cadvisor-signalfx"
	spec:
	  replicas: 1
	  selector:
	    app: "cadvisor-signalfx"
	  template:
	    metadata:
	      name: "cadvisor-signalfx"
	      labels:
	        app: "cadvisor-signalfx"
	    spec:
	      containers:
	      - name: "cadvisor-signalfx"
	        image: "quay.io/signalfx/prometheustosfx:latest"
	        env:
	      - name: SFX_SCRAPPER_API_TOKEN
	        value: <API TOKEN>
	      - name: SFX_SCRAPPER_CLUSTER_NAME
	        value: <CLUSTER NAME>
	      - name: SFX_SCRAPPER_SEND_RATE
	        value: 5s

**Step 2** - Deploy to your cluster. e.g. `kubectl create -f cadvisor-signalfx.yaml`
	      
## Full options list

| Option | Default val. | Comment | Env. Var. |
| ------ | ------------ | ------- | --------- |
| --ingestURL | "https://ingest.signalfx.com"  | The SignalFx ingest URL. | $SFX_SCRAPPER_INGEST_URL |
| --apiToken |   | The SignalFx API token. | $SFX_SCRAPPER_API_TOKEN |
| --clusterName | | The dimension name for this kubernetes cluster.  | $SFX_SCRAPPER_CLUSTER_NAME |
| --cadvisorPort | 4194  | The port on which the kubernetes cAdvisor listens. | $SFX_SCRAPPER_CADVISOR_PORT |
| --sendRate | "1s"  | The rate at which data is queried from cAdvisor and sent to SignalFx. Possible values: [10s 30s 1m 5m 1h 1s 5s] | $SFX_SCRAPPER_SEND_RATE |
| --nodeServiceDiscoveryRate | "5m" | The rate at which nodes and services will be rediscovered. Possible values: [20m 3m 5m 10m 15m] | $SFX_SCRAPPER_NODE_SERVICE_DISCOVERY_RATE |

## License

This tool is licensed under the Apache License, Version 2.0. See LICENSE for full license text.
