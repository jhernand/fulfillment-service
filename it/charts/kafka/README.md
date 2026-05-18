# Kafka Helm chart

This Kafka Helm chart is intended exclusively for development and testing environments. It deploys
a single Kafka broker in KRaft mode (no ZooKeeper) with TLS enabled via cert-manager and ephemeral
storage backed by `emptyDir`. Data does not persist across pod restarts.

**Do not use this chart in production.** It uses ephemeral storage, runs a single broker, and has
no replication. Data will be lost if the pod is restarted.

## Installation

Before installing this chart you will need a working installation of cert-manager and at least one
issuer defined.

The following table lists the configurable parameters of the Kafka chart:

| Parameter                  | Description                                                   | Required | Default                                          |
|----------------------------|---------------------------------------------------------------|----------|--------------------------------------------------|
| `certs.issuerRef.kind`     | The kind of cert-manager issuer (`ClusterIssuer` or `Issuer`) | No       | `ClusterIssuer`                                  |
| `certs.issuerRef.name`     | The name of the cert-manager issuer for TLS certificates      | **Yes**  | None                                             |
| `images.kafka`             | The Kafka container image                                     | No       | `quay.io/strimzi/kafka:1.0.0-kafka-4.2.0`       |

Note that the `certs.issuerRef.name` parameter is required. For example, to install the chart in a
Kind cluster:

```bash
$ helm install kafka it/charts/kafka \
--namespace kafka \
--create-namespace \
--set certs.issuerRef.name=default-ca \
--wait
```

To uninstall it:

```bash
$ helm uninstall kafka --namespace kafka
```

## Connecting to Kafka

The chart creates a Kubernetes service named `kafka` that exposes port 9093 (TLS). To connect from
another pod in the same namespace, use `kafka:9093` as the bootstrap server. From a different
namespace, use the fully qualified service name:

```text
kafka.kafka.svc.cluster.local:9093
```

Clients must provide a TLS certificate signed by the same CA used by the Kafka server certificate
to authenticate. The connection uses mTLS for both encryption and authentication.
