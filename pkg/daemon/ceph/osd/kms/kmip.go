/*
Copyright 2022 The Rook Authors. All rights reserved.

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

package kms

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	kmip "github.com/gemalto/kmip-go"
	"github.com/gemalto/kmip-go/kmip14"
	"github.com/gemalto/kmip-go/ttlv"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

const (
	TypeKMIP = "kmip"

	// KMIP version.
	protocolMajor = 1
	protocolMinor = 4

	// kmipDefaultReadTimeout is the default read network timeout.
	kmipDefaultReadTimeout = uint8(10)

	// kmipDefaultWriteTimeout is the default write network timeout.
	kmipDefaultWriteTimeout = uint8(10)

	// cryptographicLength of the key.
	cryptographicLength = 256

	// value not credential, just configuration keys.
	//nolint:gosec
	kmipEndpoint         = "KMIP_ENDPOINT"
	kmipTLSServerName    = "TLS_SERVER_NAME"
	kmipReadTimeOut      = "READ_TIMEOUT"
	kmipWriteTimeOut     = "WRITE_TIMEOUT"
	KmipCACert           = "CA_CERT"
	KmipClientCert       = "CLIENT_CERT"
	KmipClientKey        = "CLIENT_KEY"
	KmipUniqueIdentifier = "UNIQUE_IDENTIFIER"

	// EtcKmipDir is kmip config dir.
	EtcKmipDir = "/etc/kmip"
)

var (
	kmsKMIPMandatoryTokenDetails      = []string{KmipCACert, KmipClientCert, KmipClientKey}
	kmsKMIPMandatoryConnectionDetails = []string{kmipEndpoint}
	ErrKMIPEndpointNotSet             = errors.Errorf("%s not set.", kmipEndpoint)
	ErrKMIPCACertNotSet               = errors.Errorf("%s not set.", KmipCACert)
	ErrKMIPClientCertNotSet           = errors.Errorf("%s not set.", KmipClientCert)
	ErrKMIPClientKeyNotSet            = errors.Errorf("%s not set.", KmipClientKey)
)

type kmipKMS struct {
	// standard KMIP configuration options
	endpoint     string
	tlsConfig    *tls.Config
	readTimeout  uint8
	writeTimeout uint8
}

// InitKMIP initializes the KMIP KMS.
func InitKMIP(config map[string]string) (*kmipKMS, error) {
	kms := &kmipKMS{}

	kms.endpoint = GetParam(config, kmipEndpoint)
	if kms.endpoint == "" {
		return nil, ErrKMIPEndpointNotSet
	}

	// optional
	serverName := GetParam(config, kmipTLSServerName)

	// optional
	kms.readTimeout = kmipDefaultReadTimeout
	timeout, err := strconv.Atoi(GetParam(config, kmipReadTimeOut))
	if err == nil {
		if timeout > math.MaxUint8 {
			return nil, fmt.Errorf("read timeout %d is too big", timeout)
		}
		kms.readTimeout = uint8(timeout) // nolint:gosec // G115 : already checked if too big
	}

	// optional
	kms.writeTimeout = kmipDefaultWriteTimeout
	timeout, err = strconv.Atoi(GetParam(config, kmipWriteTimeOut))
	if err == nil {
		if timeout > math.MaxUint8 {
			return nil, fmt.Errorf("read timeout %d is too big", timeout)
		}
		kms.writeTimeout = uint8(timeout) // nolint:gosec // G115 : already checked if too big
	}

	caCert := GetParam(config, KmipCACert)
	if caCert == "" {
		return nil, ErrKMIPCACertNotSet
	}

	clientCert := GetParam(config, KmipClientCert)
	if clientCert == "" {
		return nil, ErrKMIPClientCertNotSet
	}

	clientKey := GetParam(config, KmipClientKey)
	if clientKey == "" {
		return nil, ErrKMIPClientKeyNotSet
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM([]byte(caCert))
	cert, err := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
	if err != nil {
		return nil, fmt.Errorf("invalid X509 key pair: %w", err)
	}

	kms.tlsConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName,
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}

	return kms, nil
}

// IsKMIP determines whether the configured KMS is KMIP.
func (c *Config) IsKMIP() bool { return c.Provider == TypeKMIP }

// registerKey will create a register key and return its unique identifier.
func (kms *kmipKMS) registerKey(keyName, keyValue string) (string, error) {
	valueBytes, err := base64.StdEncoding.DecodeString(keyValue)
	if err != nil {
		return "", errors.Wrap(err, "failed to convert string to bytes")
	}
	conn, err := kms.connect()
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to kmip kms")
	}
	defer conn.Close()
	registerPayload := kmip.RegisterRequestPayload{
		ObjectType: kmip14.ObjectTypeSymmetricKey,
		SymmetricKey: &kmip.SymmetricKey{
			KeyBlock: kmip.KeyBlock{
				KeyFormatType: kmip14.KeyFormatTypeOpaque,
				KeyValue: &kmip.KeyValue{
					KeyMaterial: valueBytes,
				},
				CryptographicLength:    cryptographicLength,
				CryptographicAlgorithm: kmip14.CryptographicAlgorithmAES,
			},
		},
	}
	registerPayload.TemplateAttribute.Append(kmip14.TagCryptographicUsageMask, kmip14.CryptographicUsageMaskExport)
	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn, kmip14.OperationRegister, registerPayload)
	if err != nil {
		return "", errors.Wrap(err, "failed to send register request to kmip")
	}
	bi, err := kms.verifyResponse(respMsg, kmip14.OperationRegister, uniqueBatchItemID)
	if err != nil {
		return "", errors.Wrap(err, "failed to verify kmip register response")
	}

	var registerRespPayload kmip.RegisterResponsePayload
	err = decoder.DecodeValue(&registerRespPayload, bi.ResponsePayload.(ttlv.TTLV))
	if err != nil {
		return "", errors.Wrap(err, "failed to decode kmip response value")
	}

	return registerRespPayload.UniqueIdentifier, nil
}

func (kms *kmipKMS) getKey(uniqueIdentifier string) (string, error) {
	conn, err := kms.connect()
	if err != nil {
		return "", errors.Wrap(err, "failed to connect to kmip kms")
	}
	defer conn.Close()

	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn, kmip14.OperationGet, kmip.GetRequestPayload{
		UniqueIdentifier: uniqueIdentifier,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to send get request to kmip")
	}
	bi, err := kms.verifyResponse(respMsg, kmip14.OperationGet, uniqueBatchItemID)
	if err != nil {
		return "", errors.Wrap(err, "failed to verify kmip response")
	}
	var getRespPayload kmip.GetResponsePayload
	err = decoder.DecodeValue(&getRespPayload, bi.ResponsePayload.(ttlv.TTLV))
	if err != nil {
		return "", errors.Wrap(err, "failed to decode kmip response value")
	}

	secretBytes := getRespPayload.SymmetricKey.KeyBlock.KeyValue.KeyMaterial.([]byte)
	secretBase64 := base64.StdEncoding.EncodeToString(secretBytes)

	return secretBase64, nil
}

func (kms *kmipKMS) deleteKey(uniqueIdentifier string) error {
	conn, err := kms.connect()
	if err != nil {
		return errors.Wrap(err, "failed to connect to kmip kms")
	}
	defer conn.Close()

	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn, kmip14.OperationDestroy, kmip.DestroyRequestPayload{
		UniqueIdentifier: uniqueIdentifier,
	})
	if err != nil {
		return errors.Wrap(err, "failed to send delete request to kmip")
	}
	bi, err := kms.verifyResponse(respMsg, kmip14.OperationDestroy, uniqueBatchItemID)
	if err != nil {
		return errors.Wrap(err, "failed to verify kmip response")
	}
	var destroyRespPayload kmip.DestroyResponsePayload
	err = decoder.DecodeValue(&destroyRespPayload, bi.ResponsePayload.(ttlv.TTLV))
	if err != nil {
		return errors.Wrap(err, "failed to decode kmip response value")
	}

	return nil
}

// connect to the kmip endpoint, perform TLS and KMIP handshakes.
func (kms *kmipKMS) connect() (*tls.Conn, error) {
	conn, err := tls.Dial("tcp", kms.endpoint, kms.tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to dial kmip connection endpoint: %w", err)
	}
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()
	if kms.readTimeout != 0 {
		err = conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(kms.readTimeout)))
		if err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}
	}
	if kms.writeTimeout != 0 {
		err = conn.SetReadDeadline(time.Now().Add(time.Second * time.Duration(kms.writeTimeout)))
		if err != nil {
			return nil, fmt.Errorf("failed to set write deadline: %w", err)
		}
	}

	err = conn.Handshake()
	if err != nil {
		return nil, fmt.Errorf("failed to perform connection handshake: %w", err)
	}

	err = kms.discover(conn)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// discover performs KMIP discover operation.
// https://docs.oasis-open.org/kmip/spec/v1.4/kmip-spec-v1.4.html
// chapter 4.26.
func (kms *kmipKMS) discover(conn io.ReadWriter) error {
	respMsg, decoder, uniqueBatchItemID, err := kms.send(conn,
		kmip14.OperationDiscoverVersions,
		kmip.DiscoverVersionsRequestPayload{
			ProtocolVersion: []kmip.ProtocolVersion{
				{
					ProtocolVersionMajor: protocolMajor,
					ProtocolVersionMinor: protocolMinor,
				},
			},
		})
	if err != nil {
		return err
	}

	batchItem, err := kms.verifyResponse(
		respMsg,
		kmip14.OperationDiscoverVersions,
		uniqueBatchItemID)
	if err != nil {
		return err
	}

	ttlvPayload, ok := batchItem.ResponsePayload.(ttlv.TTLV)
	if !ok {
		return errors.New("failed to parse responsePayload")
	}

	var respDiscoverVersionsPayload kmip.DiscoverVersionsResponsePayload
	err = decoder.DecodeValue(&respDiscoverVersionsPayload, ttlvPayload)
	if err != nil {
		return err
	}

	if len(respDiscoverVersionsPayload.ProtocolVersion) != 1 {
		return fmt.Errorf("invalid len of discovered protocol versions %v expected 1",
			len(respDiscoverVersionsPayload.ProtocolVersion))
	}
	pv := respDiscoverVersionsPayload.ProtocolVersion[0]
	if pv.ProtocolVersionMajor != protocolMajor || pv.ProtocolVersionMinor != protocolMinor {
		return fmt.Errorf("invalid discovered protocol version %v.%v expected %v.%v",
			pv.ProtocolVersionMajor, pv.ProtocolVersionMinor, protocolMajor, protocolMinor)
	}

	return nil
}

// send sends KMIP operation over tls connection, returns
// kmip response message,
// ttlv Decoder to decode message into desired format,
// batchItem ID,
// and error.
func (kms *kmipKMS) send(
	conn io.ReadWriter,
	operation kmip14.Operation,
	payload interface{},
) (*kmip.ResponseMessage, *ttlv.Decoder, []byte, error) {
	biID := uuid.New()

	msg := kmip.RequestMessage{
		RequestHeader: kmip.RequestHeader{
			ProtocolVersion: kmip.ProtocolVersion{
				ProtocolVersionMajor: protocolMajor,
				ProtocolVersionMinor: protocolMinor,
			},
			BatchCount: 1,
		},
		BatchItem: []kmip.RequestBatchItem{
			{
				UniqueBatchItemID: biID[:],
				Operation:         operation,
				RequestPayload:    payload,
			},
		},
	}

	req, err := ttlv.Marshal(msg)
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to ttlv marshal message: %w", err)
	}

	_, err = conn.Write(req)
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to write request onto connection: %w", err)
	}

	decoder := ttlv.NewDecoder(bufio.NewReader(conn))
	resp, err := decoder.NextTTLV()
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to read ttlv KMIP value: %w", err)
	}

	var respMsg kmip.ResponseMessage
	err = decoder.DecodeValue(&respMsg, resp)
	if err != nil {
		return nil, nil, nil,
			fmt.Errorf("failed to decode response value: %w", err)
	}

	return &respMsg, decoder, biID[:], nil
}

// verifyResponse verifies the response success and return the batch item.
func (kms *kmipKMS) verifyResponse(
	respMsg *kmip.ResponseMessage,
	operation kmip14.Operation,
	uniqueBatchItemID []byte,
) (*kmip.ResponseBatchItem, error) {
	if respMsg.ResponseHeader.BatchCount != 1 {
		return nil, fmt.Errorf("batch count %q should be \"1\"",
			respMsg.ResponseHeader.BatchCount)
	}
	if len(respMsg.BatchItem) != 1 {
		return nil, fmt.Errorf("batch Intems list len %q should be \"1\"",
			len(respMsg.BatchItem))
	}
	batchItem := respMsg.BatchItem[0]
	if operation != batchItem.Operation {
		return nil, fmt.Errorf("unexpected operation, real %q expected %q",
			batchItem.Operation, operation)
	}
	if !bytes.Equal(uniqueBatchItemID, batchItem.UniqueBatchItemID) {
		return nil, fmt.Errorf("unexpected uniqueBatchItemID, real %q expected %q",
			batchItem.UniqueBatchItemID, uniqueBatchItemID)
	}
	if kmip14.ResultStatusSuccess != batchItem.ResultStatus {
		return nil, fmt.Errorf("unexpected result status %q expected success %q,"+
			"result reason %q, result message %q",
			batchItem.ResultStatus, kmip14.ResultStatusSuccess,
			batchItem.ResultReason, batchItem.ResultMessage)
	}

	return &batchItem, nil
}
