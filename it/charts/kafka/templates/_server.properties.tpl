{{/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
specific language governing permissions and limitations under the License.
*/}}

{{- define "kafka.server.properties" -}}
# KRaft mode settings:
process.roles=broker,controller
node.id=0
controller.quorum.voters=0@localhost:9094
cluster.id=osac

# Listener configuration:
listeners=PLAINTEXT://localhost:9092,SASL_SSL://0.0.0.0:8000,CONTROLLER://localhost:9094
advertised.listeners=PLAINTEXT://localhost:9092,SASL_SSL://{{ include "kafka.hostname" . }}:8000
listener.security.protocol.map=PLAINTEXT:PLAINTEXT,SASL_SSL:SASL_SSL,CONTROLLER:PLAINTEXT
controller.listener.names=CONTROLLER
inter.broker.listener.name=PLAINTEXT

# TLS configuration (encryption only, authentication is via SASL):
ssl.keystore.type=PKCS12
ssl.keystore.location=/secrets/cert/keystore.p12
ssl.keystore.password=kafka
ssl.truststore.type=PKCS12
ssl.truststore.location=/secrets/cert/truststore.p12
ssl.truststore.password=kafka

# SASL/SCRAM-SHA-512 configuration:
sasl.enabled.mechanisms=SCRAM-SHA-512
listener.name.sasl_ssl.sasl.enabled.mechanisms=SCRAM-SHA-512
listener.name.sasl_ssl.scram-sha-512.sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="{{ .Values.sasl.username }}" password="{{ .Values.sasl.password }}";

# Log directories (ephemeral):
log.dirs=/data/kafka

# Single broker settings:
offsets.topic.replication.factor=1
transaction.state.log.replication.factor=1
transaction.state.log.min.isr=1

# Default topic settings:
auto.create.topics.enable=true
num.partitions=1
default.replication.factor=1
{{- end -}}
