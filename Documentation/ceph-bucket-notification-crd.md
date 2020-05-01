# Ceph bucket notifications CRD

## Overview

In its new releases, Ceph has introduced the bucket notifications feature. It allows to send messages to various endpoints when a new event occurs on a bucket (ref: <https://docs.ceph.com/docs/master/radosgw/notifications/)>

Setup of those notifications are normally done by sending HTTP requests to the RGW, either to create/delete topics pointing to specific endpoints, or create/delete bucket notifications based on those topics.

This functionality eases this process by avoiding to use external tools or scripts. It is replaced by creation of CR definitions that  contain all the information necessary to create topics and/or notifications, which the rook operator processes.

## Goals

Creates a CRD for topics and a CRD for notifications, defining all the necessary and optional information for the various endpoints.

Extends the rook operator to handle the CRs that would be submitted by users.

## Implementation

The CR for a topic configuration takes this form:

```yaml
apiVersion: ceph.rook.io/v1
kind: Topic
metadata:
  name: rook-ceph
  namespace: rook-ceph
Spec:
  pushEndpoint: Kafka/HTTP/RabbitMQ, mandatory
  ackLevel: none/broker, optional (default - broker)
  accessKey: S3 user access key  
  secretKey: S3 user secret key
  endpointUrl: S3 service endpoint
```

The CR for bucket notification takes this form:

```yaml
apiVersion: ceph.rook.io/v1
kind: NotificationConfiguration
metadata:
  name: rook-ceph
  namespace: rook-ceph
Spec:
  Topic: reference to the topic, topic_arn, mandatory
  Filter: Prefix/Suffix/Regex/Metadata/Tags, optional (default - {})
    Metadata:
    - name: color
      value: blue
    Prefix:
    - name: prefix
      value: *.png
  Events: s3:ObjectCreated:*/s3:ObjectRemoved:*, mandatory
  accessKey: S3 user access key  
  secretKey: S3 user secret key
  endpointUrl: S3 service endpoint
```
