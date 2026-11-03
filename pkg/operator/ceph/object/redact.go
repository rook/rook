/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package object

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// withheldMarker reports fields a document lost on its way into a log. As an object member its
// value is the number of that object's own members the allow list did not permit; as an array
// element it stands in for an element that was dropped whole. It cannot be forged by the
// document being filtered: every name in the output comes from the allow list, which never
// names it.
const withheldMarker = "<withheld>"

// logAllowList names the fields of a document that may appear in a log line. Filtering is by
// path, not by name: a field is permitted only where the list places it, so a member name
// permitted inside one object does not admit the same name inside another. Anything the list
// does not name is dropped, whatever it is called - which is the point, since a credential a
// newer Ceph adds under a name nothing here knows about is dropped by that same default.
type logAllowList struct {
	// members permits object members by the name the schema gives them. A nil value permits the
	// member as it stands, provided it is a scalar.
	members map[string]*logAllowList
	// anyMember permits every member of an object whose member names are data rather than
	// schema, such as a storage class map keyed by storage class name.
	anyMember *logAllowList
	// elements permits the elements of an array.
	elements *logAllowList
}

// isLeaf reports whether allow describes a position holding a value rather than a structure.
func (allow *logAllowList) isLeaf() bool {
	return allow == nil || (allow.members == nil && allow.anyMember == nil && allow.elements == nil)
}

// zoneLogAllowList is the allow list for a radosgw-admin zone document. It is built once;
// nothing mutates it, and filtering only reads it.
var zoneLogAllowList = sync.OnceValue(buildZoneLogAllowList)

func buildZoneLogAllowList() *logAllowList {
	// placement_pools entries are {key, val} objects. "key" and these members of "val" are what
	// shared_pools.go reads and writes through getObjProperty/updateObjProperty, and what
	// ZonePlacementPoolVal decodes - inline_data included, which rook sets explicitly to work
	// around https://tracker.ceph.com/issues/67933. default_placement and placement_targets,
	// which shared_pools.go reads through the same calls, are members of the zonegroup
	// document rather than of this one.
	placementVal := &logAllowList{
		members: map[string]*logAllowList{
			"index_pool":      nil,
			"data_extra_pool": nil,
			"inline_data":     nil,
			"storage_classes": {
				// keyed by storage class name, which is data: the CRD names the classes
				anyMember: &logAllowList{
					members: map[string]*logAllowList{"data_pool": nil},
				},
			},
		},
	}

	allow := &logAllowList{
		members: map[string]*logAllowList{
			// which zone, and which realm, a line is about
			"id":       nil,
			"name":     nil,
			"realm_id": nil,
			"placement_pools": {
				elements: &logAllowList{
					members: map[string]*logAllowList{
						"key": nil,
						"val": placementVal,
					},
				},
			},
		},
	}

	// the pool fields rook namespaces, taken from the one map that defines them: a pool field a
	// newer Ceph adds becomes loggable when it is added to zonePoolNSSuffix, and not before
	for poolField := range ZoneJsonPoolKeys() {
		allow.members[poolField] = nil
	}

	return allow
}

// filterForLog returns value reduced to what allow permits, and whether anything of it may be
// logged at all. An object reports the number of its own members that were dropped under
// withheldMarker; an array keeps the position of a dropped element, marked. A value whose shape
// does not match the one allow describes is dropped rather than guessed at: a credential can
// hide in a shape as easily as under a name.
func filterForLog(value any, allow *logAllowList) (any, bool) {
	switch {
	case allow.isLeaf():
		// a leaf permits the value as it stands, but only a scalar is one: an object or an
		// array where the schema promised a pool name is a shape this list cannot vouch for
		switch value.(type) {
		case map[string]any, []any:
			return nil, false
		default:
			return value, true
		}

	case allow.elements != nil:
		array, ok := value.([]any)
		if !ok {
			return nil, false
		}
		filtered := make([]any, 0, len(array))
		for _, element := range array {
			if f, keep := filterForLog(element, allow.elements); keep {
				filtered = append(filtered, f)
			} else {
				filtered = append(filtered, withheldMarker)
			}
		}
		return filtered, true

	default:
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		filtered := make(map[string]any, len(object))
		withheld := 0
		for name, member := range object {
			memberAllow, named := allow.members[name]
			if !named {
				if allow.anyMember == nil {
					withheld++
					continue
				}
				memberAllow = allow.anyMember
			}
			f, keep := filterForLog(member, memberAllow)
			if !keep {
				withheld++
				continue
			}
			filtered[name] = f
		}
		if withheld != 0 {
			filtered[withheldMarker] = withheld
		}
		return filtered, true
	}
}

// RedactZoneJSON returns the radosgw-admin zone document in jsonStr reduced to the fields rook
// reads from it, so a log line keeps what it is there for while the credentials the document
// carries - the realm system user's live S3 credentials, good for every bucket in the realm,
// and a cloud tier's credentials for the remote it replicates to - never reach the log. They
// are withheld because nothing permits them, not because anything enumerates them, so a
// credential field a newer Ceph adds is withheld too.
//
// What a reader loses is every field rook does not consume: a zone document is printed with
// holes in it, and a field that turns out to matter has to be added to the allow list before a
// log will show it. Each object says how many of its own members were withheld, so a reader
// can tell a document was trimmed from one that had nothing else in it.
//
// A payload that does not decode as a JSON object is reported by its size alone: a truncated
// document can end part way through a credential, where no textual match would find it.
//
// Filtering unmarshals the document and re-encodes it, so a caller logging at DEBUG guards
// the call behind logger.LevelAt: log.NamespacedDebug formats its message before capnslog
// sees the level, which would otherwise pay for the filtering on a line nothing emits.
func RedactZoneJSON(jsonStr string) string {
	var document any
	if err := json.Unmarshal([]byte(jsonStr), &document); err != nil {
		return fmt.Sprintf("<undecodable zone json redacted (%d bytes)>", len(jsonStr))
	}

	filtered, ok := filterForLog(document, zoneLogAllowList())
	if !ok {
		// a bare null, an array, a quoted string - nothing a zone document can be
		return fmt.Sprintf("<non-object zone json redacted (%d bytes)>", len(jsonStr))
	}

	var redacted bytes.Buffer
	encoder := json.NewEncoder(&redacted)
	// the default html escaping turns the marker's angle brackets into numeric escapes, which
	// is not something a person reading a log line should have to decode
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(filtered); err != nil {
		// unreachable in practice: every value json.Unmarshal produces marshals back
		return fmt.Sprintf("<unmarshalable zone json redacted (%d bytes)>", len(jsonStr))
	}
	return strings.TrimSuffix(redacted.String(), "\n")
}
