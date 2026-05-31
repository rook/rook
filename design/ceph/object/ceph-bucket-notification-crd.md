# Ceph bucket notifications CRD

## Overview

Ceph added support for the bucket notifications feature from Nautilus onwards. It allows sending messages to various endpoints when a new event occurs on a bucket [ref](https://docs.ceph.com/docs/master/radosgw/notifications/)

Setup of those notifications are normally done by sending HTTP requests to the RGW, either to create/delete topics pointing to specific endpoints, or create/delete bucket notifications based on those topics.

This functionality eases this process by avoiding to use external tools or scripts. It is replaced by creation of CR definitions that contain all the information necessary to create topics and/or notifications, which the rook operator processes.

## Goals

Creates a CRD for topics and a CRD for notifications, defining all the necessary and optional information for the various endpoints.

Extends the rook operator to handle the CRs that would be submitted by users.

## Implementation

The CR for a topic configuration takes this form:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBucketTopic
metadata:
  name: # name of the topic
  namespace: # namespace where topic belongs
spec:
  opaqueData: #(optional) opaque data is set in the topic configuration
  persistent: false #(optional) indication whether notifications to this endpoint are persistent or not (`false` by default)
  endpoint: #(mandatory) must contain exactly one of the following options
    http:
      uri: #(mandatory) URI of an endpoint to send push notification to
      disableVerifySSL: false #(optional) indicate whether the server certificate is validated by the client or not (`false` by default)
    amqp:
      uri: #(mandatory) URI of an endpoint to send push notification to
      disableVerifySSL: false #(optional) indicate whether the server certificate is validated by the client or not (`false` by default)
      caLocation: <filepath in rgw pod> #(optional) this specified CA will be used, instead of the default one, to authenticate the broker
      ackLevel: broker #(optional) none/routable/broker, optional (default - broker)
      amqpExchange: direct #(mandatory) exchanges must exist and be able to route messages based on topics
    kafka:
      uri: #(mandatory) URI of an endpoint to send push notification to
      disableVerifySSL: false #(optional) indicate whether the server certificate is validated by the client or not (`false` by default)
      useSSL: true #(optional) secure connection will be used for connecting with the broker (`false` by default)
      caLocation: <filepath in rgw pod> #(optional) this specified CA will be used, instead of the default one, to authenticate the broker
      ackLevel: broker #(optional) none/broker, optional (default - broker)
```
P.S : URI can be of different format depends on the server
- http -> `http[s]://<fqdn>[:<port][/resource]`
- amqp -> `amqp://[<user>:<password>@]<fqdn>[:<port>][/<vhost>]`
- kafka -> `kafka://[<user>:<password>@]<fqdn>[:<port]`

The CR for bucket notification takes this form:

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBucketNotification
metadata:
  name: # name of the notification
  namespace: # namespace where notification belongs
spec:
  topic: #(mandatory) reference to the topic, topic_arn
  filter: #(optional) Prefix/Suffix/Regex, optional (default - {})
    stringMatch:
    - name: prefix
      value: hello
    - name: suffix
      value: .png
    - name: regex
      value: [a-z]*

  events: # applicable values [here](https://docs.ceph.com/en/latest/radosgw/s3-notification-compatibility/#event-types), (default all)
```
The information about bucket notification can passed to OBC/BAR(from [COSI](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/1979-object-storage-support)) as labels. It can be set using `kubectl` commands, so the name of bucket notifications need to satisfy the [label syntax](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set). For OBC it will look like the following:

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-bucket
  labels:
    bucket-notification: ignored # no name is appended
    bucket-notification-name-1: name-1
    bucket-notification-name-2: name-2
    bucket-notification-foo: foo
spec:
  bucketName: mybucket
  storageClassName: rook-ceph-delete-bucket
```
Usually bucket notification will be created by user for consuming it for the applications, so it need to created on App's namespace similar to OBC/BAR.
