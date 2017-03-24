/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package operator

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/rook/rook/pkg/operator/cluster"

	"k8s.io/client-go/pkg/api/unversioned"
	kwatch "k8s.io/client-go/pkg/watch"
)

type clusterEvent struct {
	Type   kwatch.EventType
	Object *cluster.Cluster
}

type poolEvent struct {
	Type   kwatch.EventType
	Object *cluster.PoolSpec
}

type rawEvent struct {
	Type   kwatch.EventType
	Object json.RawMessage
}

func pollClusterEvent(decoder *json.Decoder) (*clusterEvent, *unversioned.Status, error) {
	re, status, err := pollEvent(decoder)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to poll cluster event. %+v", err)
	}

	ev := &clusterEvent{
		Type:   re.Type,
		Object: &cluster.Cluster{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to unmarshal Cluster object from data (%s): %v", re.Object, err)
	}
	return ev, status, nil
}

func pollPoolEvent(decoder *json.Decoder) (*poolEvent, *unversioned.Status, error) {
	re, status, err := pollEvent(decoder)
	if err != nil {
		return nil, status, fmt.Errorf("failed to poll cluster event. %+v", err)
	}

	ev := &poolEvent{
		Type:   re.Type,
		Object: &cluster.PoolSpec{},
	}
	err = json.Unmarshal(re.Object, ev.Object)
	if err != nil {
		return nil, nil, fmt.Errorf("fail to unmarshal Cluster object from data (%s): %v", re.Object, err)
	}
	return ev, nil, nil
}

func pollEvent(decoder *json.Decoder) (*rawEvent, *unversioned.Status, error) {
	re := &rawEvent{}
	err := decoder.Decode(re)
	if err != nil {
		if err == io.EOF {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("fail to decode raw event from apiserver (%v)", err)
	}

	if re.Type == kwatch.Error {
		status := &unversioned.Status{}
		err = json.Unmarshal(re.Object, status)
		if err != nil {
			return nil, nil, fmt.Errorf("fail to decode (%s) into unversioned.Status (%v)", re.Object, err)
		}
		return nil, status, nil
	}

	return re, nil, nil
}
