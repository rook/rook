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
package util

type Set struct {
	values map[string]struct{}
}

// Create a new empty set
func NewSet() *Set {
	set := &Set{}
	set.values = make(map[string]struct{})
	return set
}

// Create a new set from the array
func CreateSet(values []string) *Set {
	set := &Set{}
	set.values = make(map[string]struct{})
	for _, value := range values {
		set.add(value)
	}
	return set
}

// Create a copy of the set
func (s *Set) Copy() *Set {
	set := NewSet()
	for value := range s.values {
		set.values[value] = struct{}{}
	}

	return set
}

// Subtract the subset from the set
func (s *Set) Subtract(subset *Set) {
	// Iterate over each element in the set to see if it's in the subset
	for value := range s.values {
		if _, ok := subset.values[value]; ok {
			delete(s.values, value)
		}
	}
}

// Add a value to the set. Returns true if the value was added, false if it already exists.
func (s *Set) Add(newValue string) bool {
	if _, ok := s.values[newValue]; !ok {
		s.add(newValue)
		return true
	}

	// The value is already in the set
	return false
}

// Add a value to the set. Returns true if the value was added, false if it already exists.
func (s *Set) Remove(oldValue string) bool {
	if _, ok := s.values[oldValue]; ok {
		delete(s.values, oldValue)
		return true
	}

	// The value is not in the set
	return false
}

// Add the value to the set
func (s *Set) add(value string) {
	s.values[value] = struct{}{}
}

// Check whether a value is already contained in the set
func (s *Set) Contains(value string) bool {
	_, ok := s.values[value]
	return ok
}

// Iterate over the items in the set
func (s *Set) Iter() <-chan string {
	channel := make(chan string)
	go func() {
		for value := range s.values {
			channel <- value
		}
		close(channel)
	}()
	return channel
}

// Get the count of items in the set
func (s *Set) Count() int {
	return len(s.values)
}

// Add other set items
func (s *Set) AddSet(other *Set) {
	for value := range other.Iter() {
		s.add(value)
	}
}

// Add multiple items more efficiently
func (s *Set) AddMultiple(values []string) {
	for _, value := range values {
		s.add(value)
	}
}

// Check if two sets contain the same elements
func (s *Set) Equals(other *Set) bool {
	if s.Count() != other.Count() {
		return false
	}

	for value := range s.Iter() {
		if !other.Contains(value) {
			return false
		}
	}

	return true
}

// Convert the set to an array
func (s *Set) ToSlice() []string {
	values := []string{}
	for value := range s.values {
		values = append(values, value)
	}

	return values
}

// find items in the left slice that are not in the right slice
func SetDifference(left, right []string) *Set {
	result := NewSet()
	for _, leftItem := range left {
		foundItem := false

		// search for the left item in the right set
		for _, rightItem := range right {
			if leftItem == rightItem {
				foundItem = true
				break
			}
		}

		if !foundItem {
			result.Add(leftItem)
		}
	}

	return result
}
