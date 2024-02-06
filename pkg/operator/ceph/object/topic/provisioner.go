/*
Copyright 2021 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package topic to manage a rook bucket topics.
package topic

import (
	"context"
	"crypto/hmac"

	//nolint:gosec // sha1 is needed for v2 signatures
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	awsrequest "github.com/aws/aws-sdk-go/aws/request"
	awssession "github.com/aws/aws-sdk-go/aws/session"
	awsv4signer "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/object"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type provisioner struct {
	client           client.Client
	context          *clusterd.Context
	clusterInfo      *cephclient.ClusterInfo
	clusterSpec      *cephv1.ClusterSpec
	opManagerContext context.Context
}

// A new client type is needed here since topic management is part of AWS's Simple Notification Service (SNS) and not part of S3
func createSNSClient(p provisioner, objectStoreName types.NamespacedName) (*sns.SNS, error) {
	objStore, err := p.context.RookClientset.CephV1().CephObjectStores(objectStoreName.Namespace).Get(p.opManagerContext, objectStoreName.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get CephObjectStore %v", objectStoreName)
	}

	objContext, err := object.NewMultisiteContext(p.context, p.clusterInfo, objStore)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get object context for CephObjectStore %v", objectStoreName)
	}

	accessKey, secretKey, err := object.GetAdminOPSUserCredentials(objContext, &objStore.Spec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Ceph RGW admin ops user credentials")
	}

	// pass log level to AWS session
	logLevel := aws.LogOff
	if logger.LevelAt(capnslog.DEBUG) {
		logLevel = aws.LogDebugWithHTTPBody
	}

	// pass TLS indication and certificates to AWS session
	client := http.Client{
		Timeout: object.HttpTimeOut,
	}
	tlsEnabled := objStore.Spec.IsTLSEnabled()
	if tlsEnabled {
		tlsCert, _, err := object.GetTlsCaCert(objContext, &objStore.Spec)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get TLS certificate for the object store")
		}
		if len(tlsCert) > 0 {
			client.Transport = object.BuildTransportTLS(tlsCert, false)
		}
	}

	sess, err := awssession.NewSession(
		aws.NewConfig().
			WithRegion(object.CephRegion).
			WithCredentials(credentials.NewStaticCredentials(accessKey, secretKey, "")).
			WithEndpoint(objContext.Endpoint).
			WithMaxRetries(3).
			WithDisableSSL(!tlsEnabled).
			WithHTTPClient(&client).
			WithLogLevel(logLevel),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create a new session for CephBucketTopic provisioning with %q", objectStoreName)
	}

	logger.Debugf("session created. endpoint %q secure %v",
		*sess.Config.Endpoint,
		tlsEnabled,
	)
	snsClient := sns.New(sess)
	// This is a hack to workaround the following RGW issue: https://tracker.ceph.com/issues/50039
	// note that using: "github.com/aws/aws-sdk-go/private/signer/v2"
	// * would add the signature to the query and not the header
	// * use sha246 and not sha1
	customSignername := "cephV2.SignRequestHandler"
	snsClient.Handlers.Sign.Swap(awsv4signer.SignRequestHandler.Name, awsrequest.NamedHandler{
		Name: customSignername,
		Fn: func(req *awsrequest.Request) {
			credentials, err := req.Config.Credentials.Get()
			if err != nil {
				logger.Debugf("%s failed to get credentials: %v", customSignername, err)
				return
			}
			date := req.Time.UTC().Format(time.RFC1123Z)
			contentType := "application/x-www-form-urlencoded; charset=utf-8"
			stringToSign := req.HTTPRequest.Method + "\n\n" + contentType + "\n" + date + "\n" + req.HTTPRequest.URL.Path
			hash := hmac.New(sha1.New, []byte(credentials.SecretAccessKey))
			hash.Write([]byte(stringToSign))
			signature := base64.StdEncoding.EncodeToString(hash.Sum(nil))
			if len(req.HTTPRequest.Header["Authorization"]) == 0 {
				req.HTTPRequest.Header.Add("Authorization", "AWS "+credentials.AccessKeyID+":"+signature)
			}
			if len(req.HTTPRequest.Header["Date"]) == 0 {
				req.HTTPRequest.Header.Add("Date", date)
			}
		},
	})
	return snsClient, nil
}

func createTopicAttributes(topic *cephv1.CephBucketTopic) map[string]*string {
	attr := make(map[string]*string)
	nsName := types.NamespacedName{Name: topic.Name, Namespace: topic.Namespace}

	attr["OpaqueData"] = &topic.Spec.OpaqueData
	persistent := strconv.FormatBool(topic.Spec.Persistent)
	attr["persistent"] = &persistent
	var verifySSL string
	var useSSL string
	if topic.Spec.Endpoint.AMQP != nil {
		logger.Infof("creating CephBucketTopic %q with endpoint %q", nsName, topic.Spec.Endpoint.AMQP.URI)
		attr["push-endpoint"] = &topic.Spec.Endpoint.AMQP.URI
		attr["amqp-exchange"] = &topic.Spec.Endpoint.AMQP.Exchange
		attr["amqp-ack-level"] = &topic.Spec.Endpoint.AMQP.AckLevel
		verifySSL = strconv.FormatBool(!topic.Spec.Endpoint.AMQP.DisableVerifySSL)
		attr["verify-ssl"] = &verifySSL
	}
	if topic.Spec.Endpoint.HTTP != nil {
		logger.Infof("creating CephBucketTopic %q with endpoint %q", nsName, topic.Spec.Endpoint.HTTP.URI)
		attr["push-endpoint"] = &topic.Spec.Endpoint.HTTP.URI
		verifySSL = strconv.FormatBool(!topic.Spec.Endpoint.HTTP.DisableVerifySSL)
		attr["verify-ssl"] = &verifySSL
		cloudEvents := strconv.FormatBool(topic.Spec.Endpoint.HTTP.SendCloudEvents)
		attr["cloudevents"] = &cloudEvents
	}
	if topic.Spec.Endpoint.Kafka != nil {
		logger.Infof("creating CephBucketTopic %q with endpoint %q", nsName, topic.Spec.Endpoint.Kafka.URI)
		attr["push-endpoint"] = &topic.Spec.Endpoint.Kafka.URI
		useSSL = strconv.FormatBool(topic.Spec.Endpoint.Kafka.UseSSL)
		attr["use-ssl"] = &useSSL
		attr["kafka-ack-level"] = &topic.Spec.Endpoint.Kafka.AckLevel
		verifySSL = strconv.FormatBool(!topic.Spec.Endpoint.Kafka.DisableVerifySSL)
		attr["verify-ssl"] = &verifySSL
	}

	return attr
}

// Allow overriding this function for unit tests
var createTopicFunc = createTopic

func createTopic(p provisioner, topic *cephv1.CephBucketTopic) (*string, error) {
	nsName := types.NamespacedName{Name: topic.Name, Namespace: topic.Namespace}

	snsClient, err := createSNSClient(p, types.NamespacedName{Name: topic.Spec.ObjectStoreName, Namespace: topic.Spec.ObjectStoreNamespace})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create SNS client for CephBucketTopic %q provisioning", nsName)
	}
	topicOutput, err := snsClient.CreateTopic(&sns.CreateTopicInput{
		Name:       &topic.Name,
		Attributes: createTopicAttributes(topic),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "failed to provision CephBucketTopic %q", nsName)
	}

	logger.Infof("CephBucketTopic %q created with ARN %q", nsName, *topicOutput.TopicArn)

	return topicOutput.TopicArn, nil
}

// Allow overriding this function for unit tests
var deleteTopicFunc = deleteTopic

func deleteTopic(p provisioner, topic *cephv1.CephBucketTopic) error {
	nsName := types.NamespacedName{Name: topic.Name, Namespace: topic.Namespace}
	logger.Infof("deleting CephBucketTopic %q", nsName)

	if topic.Status.ARN == nil {
		logger.Warningf("ignore CephBucketTopic deletion. topic %q was never successfully provisioned", nsName)
		return nil
	}

	snsClient, err := createSNSClient(p, types.NamespacedName{Name: topic.Spec.ObjectStoreName, Namespace: topic.Spec.ObjectStoreNamespace})
	if err != nil {
		return errors.Wrapf(err, "failed to create SNS client for CephBucketTopic %q deletion", nsName)
	}

	_, err = snsClient.DeleteTopic(&sns.DeleteTopicInput{TopicArn: topic.Status.ARN})

	if err != nil {
		if err.(awserr.Error).Code() != sns.ErrCodeNotFoundException {
			return errors.Wrapf(err, "failed to delete CephBucketTopic %q", nsName)
		}
		logger.Warningf("ignore CephBucketTopic deletion. %q was already deleted", nsName)
	}

	logger.Infof("CephBucketTopic %q deleted", nsName)

	return nil
}

func GetProvisioned(cl client.Client, ctx context.Context, topicName types.NamespacedName) (*cephv1.CephBucketTopic, error) {
	bucketTopic := &cephv1.CephBucketTopic{}
	if err := cl.Get(ctx, topicName, bucketTopic); err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve CephBucketTopic %q", topicName)
	}
	if bucketTopic.Status == nil || bucketTopic.Status.ARN == nil {
		return nil, errors.Errorf("no ARN in topic. CephBucketTopic %q was not provisioned yet", topicName)
	}
	topicARN := *bucketTopic.Status.ARN
	parsedTopicARN, err := arn.Parse(topicARN)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse CephBucketTopic %q ARN %q", topicName, topicARN)
	}
	if strings.ToLower(parsedTopicARN.Service) != "sns" {
		return nil, errors.Errorf("CephBucketTopic %q ARN %q must have 'sns' service", topicName, topicARN)
	}
	if parsedTopicARN.Resource == "" {
		return nil, errors.Errorf("CephBucketTopic %q is missing a topic inside ARN %q", topicName, topicARN)
	}
	logger.Debugf("CephBucketTopic %q found with valid ARN %q", topicName, topicARN)

	return bucketTopic, nil
}
