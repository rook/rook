/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package client

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	crushReplicatedType      = 1
	ruleMinSizeDefault       = 1
	ruleMaxSizeDefault       = 10
	twoStepCRUSHRuleTemplate = `
rule %s {
        id %d
        type replicated
        min_size %d
        max_size %d
        step take %s %s
        step choose firstn 0 type %s
        step chooseleaf firstn 2 type %s
        step emit
}
`
	twoStepHybridCRUSHRuleTemplate = `
rule %s {
        id %d
        type replicated
        min_size %d
        max_size %d
        step take %s class %s
        step chooseleaf firstn 1 type %s
        step emit
        step take %s class %s
        step chooseleaf firstn 0 type %s
        step emit
}
`
)

var (
	stepEmit = &stepSpec{Operation: "emit"}
)

func buildTwoStepPlainCrushRule(crushMap CrushMap, ruleName string, pool cephv1.PoolSpec) string {
	var crushRuleInsert string
	if pool.DeviceClass != "" {
		crushRuleInsert = fmt.Sprintf("class %s", pool.DeviceClass)
	}
	return fmt.Sprintf(
		twoStepCRUSHRuleTemplate,
		ruleName,
		generateRuleID(crushMap.Rules),
		ruleMinSizeDefault,
		ruleMaxSizeDefault,
		pool.CrushRoot,
		crushRuleInsert,
		pool.FailureDomain,
		pool.Replicated.SubFailureDomain,
	)
}

func buildTwoStepHybridCrushRule(crushMap CrushMap, ruleName string, pool cephv1.PoolSpec) string {
	primaryOSDDeviceClass := pool.Replicated.HybridStorage.PrimaryDeviceClass
	secondaryOSDsDeviceClass := pool.Replicated.HybridStorage.SecondaryDeviceClass

	return fmt.Sprintf(
		twoStepHybridCRUSHRuleTemplate,
		ruleName,
		generateRuleID(crushMap.Rules),
		ruleMinSizeDefault,
		ruleMaxSizeDefault,
		pool.CrushRoot,
		primaryOSDDeviceClass,
		pool.FailureDomain,
		pool.CrushRoot,
		secondaryOSDsDeviceClass,
		pool.FailureDomain,
	)
}

func buildTwoStepCrushRule(crushMap CrushMap, ruleName string, pool cephv1.PoolSpec) *ruleSpec {
	/*
		The complete CRUSH rule looks like this:

		   rule two_rep_per_dc {
		           id 1
		           type replicated
		           min_size 1
		           max_size 10
		           step take root
		           step choose firstn 0 type datacenter
		           step chooseleaf firstn 2 type host
		           step emit
		   }

	*/

	ruleID := generateRuleID(crushMap.Rules)
	return &ruleSpec{
		ID:      ruleID,
		Name:    ruleName,
		Ruleset: ruleID,
		Type:    crushReplicatedType,
		MinSize: ruleMinSizeDefault,
		MaxSize: ruleMaxSizeDefault,
		Steps:   buildTwoStepCrushSteps(pool),
	}
}

func buildTwoStepCrushSteps(pool cephv1.PoolSpec) []stepSpec {
	// Create CRUSH rule steps
	steps := []stepSpec{}

	// Create the default step, which is essentially the entrypoint, the "root" of all requests
	stepTakeDefault := &stepSpec{
		Operation: "take",
		Item:      -1,
		ItemName:  pool.CrushRoot,
	}
	steps = append(steps, *stepTakeDefault)

	// Steps two
	stepTakeFailureDomain := &stepSpec{
		Operation: "chooseleaf_firstn",
		Number:    0,
		Type:      pool.FailureDomain,
	}
	steps = append(steps, *stepTakeFailureDomain)

	// Step three
	stepTakeSubFailureDomain := &stepSpec{
		Operation: "chooseleaf_firstn",
		// nolint:gosec // G115 - casting uint to int has no overflow
		Number: int(pool.Replicated.ReplicasPerFailureDomain),
		Type:   pool.Replicated.SubFailureDomain,
	}
	steps = append(steps, *stepTakeSubFailureDomain)
	steps = append(steps, *stepEmit)

	return steps
}

func generateRuleID(rules []ruleSpec) int {
	newRulesID := rules[len(rules)-1].ID + 1

	for {
		ruleIDExists := checkIfRuleIDExists(rules, newRulesID)
		if !ruleIDExists {
			break
		} else {
			newRulesID++
		}
	}

	return newRulesID
}

func checkIfRuleIDExists(rules []ruleSpec, ID int) bool {
	for _, rule := range rules {
		if rule.ID == ID {
			return true
		}
	}

	return false
}

func getCrushRule(context *clusterd.Context, clusterInfo *ClusterInfo, name string) (ruleSpec, error) {
	var rule ruleSpec
	args := []string{"osd", "crush", "rule", "dump", name}
	buf, err := NewCephCommand(context, clusterInfo, args).Run()
	if err != nil {
		return rule, errors.Wrapf(err, "failed to get crush rule %q. %s", name, string(buf))
	}

	err = json.Unmarshal(buf, &rule)
	if err != nil {
		return rule, errors.Wrapf(err, "failed to unmarshal crush rule. %s", string(buf))
	}

	return rule, nil
}
