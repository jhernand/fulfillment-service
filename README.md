# Fulfillment service

This project contains the code for the fulfillment service.

## Required development tools

To work with this project you will need the following tools:

- [Go](https://go.dev) - Used to build the Go code.
- [Buf](https://buf.build) - Used to generate Go code from gRPC specifications.
- [Ginkgo](https://onsi.github.io/ginkgo) - Used to run unit tests.
- [Kustomize](https://kustomize.io) - Used to generate Kubernetes manifests.
- [Kubectl](https://kubernetes.io/es/docs/reference/kubectl) - Used to deploy to an OpenShift cluster.
- [PostgreSQL](https://www.postgresql.org) - Used to store persistent state.
- [Podman](https://podman.io) - Used to build and run container images.
- [gRPCurl](https://github.com/fullstorydev/grpcurl) - Used to test the gRPC API from the CLI.
- [curl](https://curl.se) - Used to test the REST API from the CLI.
- [jq](https://jqlang.org) - Used by some of the commands in this document.

## Building the binary

To build the `fulfillment-service` binary run `go build`.

## Running unit tests

To run unit the unit tests run `ginkgo run -r`.

## Running the service

To run the service you will previously need have the PostgreSQL database up and running, and create a database for the
service. For example, assuming that you already have administrator access to the database you can create a `service`
user with password `service123` and a `service` database with the following commands:

    postgres=# create user service with password 'service123';
    CREATE ROLE
    postgres=# create database service owner service;
    CREATE DATABASE
    postgres=#

To run the the gRPC server use a command like this:

    $ ./fulfillment-service  start server \
    --log-level=debug \
    --log-headers=true \
    --log-bodies=true \
    --grpc-listener-address=localhost:8000 \
    --db-url=postgres://service:service123@localhost:5432/service

To run the the REST gateway use a command like this:

    $ ./fulfillment-service start gateway \
    --log-level=debug \
    --log-headers=true \
    --log-bodies=true \
    --http-listener-address=localhost:8001 \
    --grpc-server-address=localhost:8000 \
    --grpc-server-plaintext

You may need to adjust the commands to use your database details.

To verify that the gRPC server is working use `grpcurl`. For example, to list the available gRPC services:

    $ grpcurl -plaintext localhost:8000 list
    fulfillment.v1.ClusterOrders
    fulfillment.v1.ClusterTemplates
    fulfillment.v1.Clusters
    fulfillment.v1.Events
    grpc.reflection.v1.ServerReflection
    grpc.reflection.v1alpha.ServerReflection

To list the methods available in a service, for example in the `ClusterTemplates` service:

    $ grpcurl -plaintext localhost:8000 list fulfillment.v1.ClusterTemplates
    fulfillment.v1.ClusterTemplates.Get
    fulfillment.v1.ClusterTemplates.List

To invoke a method, for example the `List` method of the `ClusterTemplates` service:

    $ grpcurl -plaintext localhost:8000 fulfillment.v1.ClusterTemplates/List
    {
      "size": 2,
      "total": 2,
      "items": [
        {
          "id": "045cbf50-a04f-4b9a-9ea5-722fd7655a24",
          "title": "my_template",
          "description": "My template is *nice*."
        },
        {
          "id": "2cf86b60-9047-45af-8e5a-efa6f92d34ae",
          "title": "your_template",
          "description": "Your template is _ugly_."
        }
      ]
    }

To verify that the REST gateway is working use `curl`. For example, to get the list of templates:

    $ curl --silent http://localhost:8001/api/fulfillment/v1/cluster_templates | jq
    {
      "size": 2,
      "total": 2,
      "items": [
        {
          "id": "045cbf50-a04f-4b9a-9ea5-722fd7655a24",
          "title": "my_template",
          "description": "My template is *nice*."
        },
        {
          "id": "2cf86b60-9047-45af-8e5a-efa6f92d34ae",
          "title": "your_template",
          "description": "Your template is _ugly_."
        }
      ]
}

## Building the container image

Select your image name, for example `quay.io/myuser/fulfillment-service:latest`, then build and tag the image with a
command like this:

    $ podman build -t quay.io/myuser/fulfillment-service:latest .

If you want to deploy to an OpenShift cluster then you will also need to push the image, so that the cluster can pull
it:

    $ podman push quay.io/myuser/fulfillment-service:latest

## Deploying to an OpenShift cluster

In order to be able to use gRPC in an OpenShift cluster it is necessary to enable HTTP/2:

    $ oc annotate ingresses.config/cluster ingress.operator.openshift.io/default-enable-http2=true

To deploy to an using the default image run the following command:

    $ kubectl apply -k manifests

To undeploy:

    $ kubectl delete -k manifests

If you want to deploy using your own image, then you will need first to edit the manifests:

    $ cd manifests
    $ kustomize edit set image fulfillment-service=quay.io/myuser/fulfillment-service:latest

To verify that the deployment is working get the URL of the route, and use `grpcurl` and `curl` to verify that both the
gRPC server and the REST gateway are working:

    $ kubectl get route -n innabox fulfillment-api -o json | jq -r '.spec.host'
    fulfillment-api-innabox.apps.mycluster.com

    $ grpcurl -insecure fulfillment-api-innabox.apps.mycluster.com:443 fulfillment.v1.ClusterTemplates/List
    {
      "size": 2,
      "total": 2,
      "items": [
        {
          "id": "045cbf50-a04f-4b9a-9ea5-722fd7655a24",
          "title": "my_template",
          "description": "My template is *nice*."
        },
        {
          "id": "2cf86b60-9047-45af-8e5a-efa6f92d34ae",
          "title": "your_template",
          "description": "Your template is _ugly_."
        }
      ]
    }

    $  curl --silent --insecure https://fulfillment-api-innabox.apps.sno.home.arpa:443/api/fulfillment/v1/cluster_templates | jq
    {
      "size": 2,
      "total": 2,
      "items": [
        {
          "id": "045cbf50-a04f-4b9a-9ea5-722fd7655a24",
          "title": "my_template",
          "description": "My template is *nice*."
        },
        {
          "id": "2cf86b60-9047-45af-8e5a-efa6f92d34ae",
          "title": "your_template",
          "description": "Your template is _ugly_."
        }
  ]
}
