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
	"time"

	"github.com/pkg/errors"
)

// ValidateMonitoringSpec checks the monitoring settings that the CRD schema cannot express.
func ValidateMonitoringSpec(spec MonitoringSpec) error {
	// Prometheus rejects a scrape timeout longer than the interval it is scraping on, so
	// catch it here rather than letting the ServiceMonitor be written and fail to load.
	// With no explicit interval the effective one is Prometheus' own global scrape
	// interval, which Rook cannot see, so there is nothing to compare against.
	if spec.Interval == nil || spec.ScrapeTimeoutSeconds <= 0 {
		return nil
	}
	scrapeTimeout := time.Duration(spec.ScrapeTimeoutSeconds) * time.Second
	if scrapeTimeout > spec.Interval.Duration {
		return errors.Errorf("scrapeTimeoutSeconds (%s) must not be greater than the scrape interval (%s)", scrapeTimeout, spec.Interval.Duration)
	}
	return nil
}
