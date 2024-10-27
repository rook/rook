---
title: Object Bucket Notifications
---

Rook supports the creation of bucket notifications via two custom resources:

* a `CephBucketNotification` is a custom resource the defines: topic, events and filters of a bucket notification, and is described by a Custom Resource Definition (CRD) shown below. Bucket notifications are associated with a bucket by setting labels on the Object Bucket claim (OBC).
See the Ceph documentation for detailed information: [Bucket Notifications - Ceph Object Gateway - Ceph Documentation](https://docs.ceph.com/en/latest/radosgw/notifications/).
* a `CephBucketTopic` is custom resource which represents a bucket notification topic and is described by a CRD shown below. A bucket notification topic represents an endpoint (or a "topic" inside this endpoint) to which bucket notifications could be sent.

## Notifications

A CephBucketNotification defines what bucket actions trigger the notification and which topic to send notifications to. A CephBucketNotification may also define a filter, based on the object's name and other object attributes. Notifications can be associated with buckets created via ObjectBucketClaims by adding labels to an ObjectBucketClaim with the following format:

```yaml
bucket-notification-<notification name>: <notification name>
```

The CephBucketTopic, CephBucketNotification and ObjectBucketClaim must all belong to the same namespace.
If a bucket was created manually (not via an ObjectBucketClaim), notifications on this bucket should also be created manually. However, topics in these notifications may reference topics that were created via CephBucketTopic resources.

## Topics

A CephBucketTopic represents an endpoint (of types: Kafka, AMQP0.9.1 or HTTP), or a specific resource inside this endpoint (e.g a Kafka or an AMQP topic, or a specific URI in an HTTP server). The CephBucketTopic also holds any additional info needed for a CephObjectStore's RADOS Gateways (RGW) to connect to the endpoint. Topics don't belong to a specific bucket or notification. Notifications from multiple buckets may be sent to the same topic, and one bucket (via multiple CephBucketNotifications) may send notifications to multiple topics.

## Notification Reliability and Delivery

Notifications may be sent synchronously, as part of the operation that triggered them. In this mode, the operation is acknowledged only after the notification is sent to the topic’s configured endpoint, which means that the round trip time of the notification is added to the latency of the operation itself.
The original triggering operation will still be considered as successful even if the notification fail with an error, cannot be delivered or times out.

Notifications may also be sent asynchronously. They will be committed into persistent storage and then asynchronously sent to the topic’s configured endpoint. In this case, the only latency added to the original operation is of committing the notification to persistent storage.
If the notification fail with an error, cannot be delivered or times out, it will be retried until successfully acknowledged.

## Example

### CephBucketTopic Custom Resource

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBucketTopic
metadata:
  name: my-topic [1]
  namespace: my-app-space [2]
spec:
  objectStoreName: my-store [3]
  objectStoreNamespace: rook-ceph [4]
  opaqueData: my@email.com [5]
  persistent: false [6]
  endpoint: [7]
    http: [8]
      uri: http://my-notification-endpoint:8080
#     uri: http://my-notification-endpoint:8080/my-topic
#     uri: https://my-notification-endpoint:8443
      disableVerifySSL: true [9]
      sendCloudEvents: false [10]
#   amqp: [11]
#     uri: amqp://my-rabbitmq-service:5672
#     uri: amqp://my-rabbitmq-service:5672/vhost1
#     uri: amqps://user@password:my-rabbitmq-service:5672
#     disableVerifySSL: true [12]
#     ackLevel: broker [13]
#     exchange: my-exchange [14]
#   kafka: [15]
#     uri: kafka://my-kafka-service:9092
#     disableVerifySSL: true [16]
#     ackLevel: broker [17]
#     useSSL: false [18]
```

1. `name` of the `CephBucketTopic`
    + In case of AMQP endpoint, the name is used for the AMQP topic (“routing key” for a topic exchange)
    + In case of Kafka endpoint, the name is used as the Kafka topic
2. `namespace`(optional) of the `CephBucketTopic`. Should match the namespace of the CephBucketNotification associated with this CephBucketTopic, and the OBC with the label referencing the CephBucketNotification
3. `objectStoreName` is the name of the object store in which the topic should be created. This must be the same object store used for the buckets associated with the notifications referencing this topic.
4. `objectStoreNamespace` is the namespace of the object store in which the topic should be created
5. `opaqueData` (optional) is added to all notifications triggered by a notifications associated with the topic
6. `persistent` (optional) indicates whether notifications to this endpoint are persistent (=asynchronous) or sent synchronously (“false” by default)
7. `endpoint` to which to send the notifications to. Exactly one of the endpoints must be defined: `http`, `amqp`, `kafka`
8. `http` (optional) hold the spec for an HTTP endpoint. The format of the URI would be: `http[s]://<fqdn>[:<port>][/<resource>]`
    + port defaults to: 80/443 for HTTP/S accordingly
9. `disableVerifySSL` indicates whether the RGW is going to verify the SSL certificate of the HTTP server in case HTTPS is used ("false" by default)
10. `sendCloudEvents`: (optional) send the notifications with the [CloudEvents header](https://github.com/cloudevents/spec/blob/main/cloudevents/adapters/aws-s3.md). ("false" by default)
11. `amqp` (optional) hold the spec for an AMQP endpoint. The format of the URI would be: `amqp[s]://[<user>:<password>@]<fqdn>[:<port>][/<vhost>]`
    + port defaults to: 5672/5671 for AMQP/S accordingly
    + user/password defaults to: guest/guest
    + user/password may only be provided if HTTPS is used with the RGW. If not, topic creation request will be rejected
    + vhost defaults to: “/”
12. `disableVerifySSL` (optional) indicates whether the RGW is going to verify the SSL certificate of the AMQP server in case AMQPS is used ("false" by default)
13. `ackLevel` (optional) indicates what kind of ack the RGW is waiting for after sending the notifications:
    + “none”: message is considered “delivered” if sent to broker
    + “broker”: message is considered “delivered” if acked by broker (default)
    + “routable”: message is considered “delivered” if broker can route to a consumer
14. `exchange` in the AMQP broker that would route the notifications. Different topics pointing to the same endpoint must use the same exchange
15. `kafka` (optional) hold the spec for a Kafka endpoint. The format of the URI would be: `kafka://[<user>:<password>@]<fqdn>[:<port]`
    + port defaults to: 9092
    + user/password may only be provided if HTTPS is used with the RGW. If not, topic creation request will be rejected
    + user/password may only be provided together with `useSSL`, if not, the connection to the broker would fail
16. `disableVerifySSL` (optional) indicates whether the RGW is going to verify the SSL certificate of the Kafka server in case `useSSL` flag is used ("false" by default)
17. `ackLevel` (optional) indicates what kind of ack the RGW is waiting for after sending the notifications:
    + “none”: message is considered “delivered” if sent to broker
    + “broker”: message is considered “delivered” if acked by broker (default)
18. `useSSL` (optional) indicates that secure connection will be used for connecting with the broker (“false” by default)

!!! note
    In case of Kafka and AMQP, the consumer of the notifications is not required to ack the notifications, since the broker persists the messages before delivering them to their final destinations.

### CephBucketNotification Custom Resource

```yaml
apiVersion: ceph.rook.io/v1
kind: CephBucketNotification
metadata:
  name: my-notification [1]
  namespace: my-app-space [2]
spec:
  topic: my-topic [3]
  filter: [4]
    keyFilters: [5]
      # match objects with keys that start with "hello"
      - name: prefix
        value: hello
      # match objects with keys that end with ".png"
      - name: suffix
        value: .png
      # match objects with keys with only lowercase characters
      - name: regex
        value: "[a-z]*\\.*"
    metadataFilters: [6]
      - name: x-amz-meta-color
        value: blue
      - name: x-amz-meta-user-type
        value: free
    tagFilters: [7]
      - name: project
        value: brown
  # notification apply for any of the events
  # full list of supported events is here:
  # https://docs.ceph.com/en/latest/radosgw/s3-notification-compatibility/#event-types
  events: [8]
    - s3:ObjectCreated:Put
    - s3:ObjectCreated:Copy
```

1. `name` of the `CephBucketNotification`
2. `namespace`(optional) of the `CephBucketNotification`. Should match the namespace of the CephBucketTopic referenced in [3], and the OBC with the label referencing the CephBucketNotification
3. `topic` to which the notifications should be sent
4. `filter` (optional) holds a list of filtering rules of different types. Only objects that match all the filters will trigger notification sending
5. `keyFilter` (optional) are filters based on the object key. There could be up to 3 key filters defined: `prefix`, `suffix` and `regex`
6. `metadataFilters` (optional) are filters based on the object metadata. All metadata fields defined as filters must exists in the object, with the values defined in the filter. Other metadata fields may exist in the object
7. `tagFilters` (optional) are filters based on object tags. All tags defined as filters must exists in the object, with the values defined in the filter. Other tags may exist in the object
8. `events` (optional) is a list of events that should trigger the notifications. By default all events should trigger notifications. Valid Events are:
    * s3:ObjectCreated:*
    * s3:ObjectCreated:Put
    * s3:ObjectCreated:Post
    * s3:ObjectCreated:Copy
    * s3:ObjectCreated:CompleteMultipartUpload
    * s3:ObjectRemoved:*
    * s3:ObjectRemoved:Delete
    * s3:ObjectRemoved:DeleteMarkerCreated

### OBC Custom Resource

For a notifications to be associated with a bucket, a labels must be added to the OBC, indicating the name of the notification.
To delete a notification from a bucket the matching label must be removed.
When an OBC is deleted, all of the notifications associated with the bucket will be deleted as well.

```yaml
apiVersion: objectbucket.io/v1alpha1
kind: ObjectBucketClaim
metadata:
  name: ceph-notification-bucket
  labels:
    # labels that don't have this structure: bucket-notification-<name> : <name>
    # are ignored by the operator's bucket notifications provisioning mechanism
    some-label: some-value
    # the following label adds notifications to this bucket
    bucket-notification-my-notification: my-notification
    bucket-notification-another-notification: another-notification
spec:
  generateBucketName: ceph-bkt
  storageClassName: rook-ceph-delete-bucket
```
