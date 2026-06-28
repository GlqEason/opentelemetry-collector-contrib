// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package processesscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processesscraper"

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// enableOpenFileDescriptors controls whether system.processes.open_file_descriptors
// is scraped.
const enableOpenFileDescriptors = true

// getProcessOpenFDs reads the system-wide open file descriptor count from
// /proc/sys/fs/file-nr in a single O(1) read. Unlike the per-process enumeration
// used on platforms without an equivalent global counter, this approach avoids
// iterating over /proc/<pid>/fd for every running process and is therefore much
// faster on systems with a large number of processes. The value is an atomic
// kernel snapshot and exactly matches the true system total.
func getProcessOpenFDs(_ context.Context) (int64, error) {
	data, err := os.ReadFile("/proc/sys/fs/file-nr")
	if err != nil {
		return 0, err
	}

	// file-nr format: <allocated> <unused> <max>
	// The first field is the number of file descriptors currently open on the system.
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, nil
	}
	v, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}
