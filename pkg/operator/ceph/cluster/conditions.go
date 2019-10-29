//Contains functions that are used to manage each Condition Types.

// Package cluster to manage a Ceph cluster.
package cluster

import (
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	v1 "k8s.io/api/core/v1"
)

func (c *ClusterController) ConditionConnecting(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnecting,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionConnected(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnected,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionIgnored(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionIgnored,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionUpgrading(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionFailure(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionProgressing,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionFalse,
		Reason:  "ClusterNotReady",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionFailure,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionProgressing(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  "",
		Message: message,
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionProgressing,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionReady(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: message,
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionUpdating(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionFalse,
		Reason:  "ClusterNotReady",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionAvailable(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func (c *ClusterController) ConditionInitialize(namespace, name string, conditions *[]cephv1.RookConditions, reason, message string) {
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnecting,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnected,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionFailure,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionIgnored,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	c.SetCondition(namespace, name, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}
