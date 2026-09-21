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

package v1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateMonitoringSpec(t *testing.T) {
	interval := func(d time.Duration) *metav1.Duration { return &metav1.Duration{Duration: d} }

	t.Run("an empty spec is valid", func(t *testing.T) {
		assert.NoError(t, ValidateMonitoringSpec(MonitoringSpec{}))
	})

	t.Run("a scrape timeout shorter than the interval is valid", func(t *testing.T) {
		assert.NoError(t, ValidateMonitoringSpec(MonitoringSpec{
			Interval:             interval(60 * time.Second),
			ScrapeTimeoutSeconds: 30,
		}))
	})

	t.Run("a scrape timeout equal to the interval is valid", func(t *testing.T) {
		assert.NoError(t, ValidateMonitoringSpec(MonitoringSpec{
			Interval:             interval(30 * time.Second),
			ScrapeTimeoutSeconds: 30,
		}))
	})

	t.Run("a scrape timeout longer than the interval is rejected", func(t *testing.T) {
		err := ValidateMonitoringSpec(MonitoringSpec{
			Interval:             interval(30 * time.Second),
			ScrapeTimeoutSeconds: 45,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must not be greater than the scrape interval")
	})

	t.Run("a scrape timeout without an interval is left to Prometheus", func(t *testing.T) {
		assert.NoError(t, ValidateMonitoringSpec(MonitoringSpec{ScrapeTimeoutSeconds: 3600}))
	})

	t.Run("an interval without a scrape timeout is valid", func(t *testing.T) {
		assert.NoError(t, ValidateMonitoringSpec(MonitoringSpec{Interval: interval(10 * time.Second)}))
	})
}
