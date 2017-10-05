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
*/
package api

import (
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rook/rook/pkg/model"
)

// Gets the nodes that are part of this cluster.
// GET
// /node
func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	logger.Infof("Getting nodes")
	nodes := []model.Node{}
	options := metav1.ListOptions{}
	nl, err := h.config.context.Clientset.CoreV1().Nodes().List(options)
	if err != nil {
		logger.Errorf("failed to get nodes. %+v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	for _, n := range nl.Items {
		node := model.Node{
			NodeID:      n.Status.NodeInfo.SystemUUID,
			PublicIP:    n.Spec.ExternalID,
			ClusterName: n.Namespace,
		}
		nodes = append(nodes, node)
	}

	FormatJsonResponse(w, nodes)
}
