// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build !linux && !darwin && !freebsd && !openbsd && !windows

package processesscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processesscraper"

import (
	"context"
)

// enableOpenFileDescriptors is false on platforms where gopsutil does not
// implement NumFDsWithContext; the metric is not emitted there.
const enableOpenFileDescriptors = false

// getProcessOpenFDs is a no-op on unsupported platforms.
func getProcessOpenFDs(context.Context) (int64, error) {
	return 0, nil
}
