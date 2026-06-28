// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || freebsd || openbsd || windows

package processesscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processesscraper"

import (
	"context"

	"github.com/shirou/gopsutil/v4/process"
)

// enableOpenFileDescriptors controls whether system.processes.open_file_descriptors
// is scraped. It is enabled on platforms where we can obtain the total count
// efficiently (Linux reads /proc/sys/fs/file-nr in O(1); Windows calls
// GetPerformanceInfo). On macOS and FreeBSD we fall back to enumerating all
// processes and summing per-process descriptors, which may be more expensive
// when many processes are running.
const enableOpenFileDescriptors = true

// procFD is the subset of gopsutil's process.Process used to gather open file
// descriptor counts. It is separate from the proc interface above so that the
// existing process-count fakes do not need to implement it.
type procFD interface {
	NumFDsWithContext(context.Context) (int32, error)
}

// getProcessOpenFDs returns the total number of open file descriptors
// (handles on Windows) across all processes. On macOS and BSDs this is done
// by enumerating all processes and summing per-process descriptor counts.
// Per-process errors (permission denied, terminated, etc.) are skipped, so
// the value may slightly underestimate the true total.
func getProcessOpenFDs(ctx context.Context) (int64, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return 0, err
	}

	procFDs := make([]procFD, len(procs))
	for i, p := range procs {
		procFDs[i] = p
	}
	return sumOpenFDs(ctx, procFDs)
}

// sumOpenFDs accumulates the open file descriptor counts of the given
// processes. A per-process error (permission denied, already terminated, etc.)
// is expected and skipped, matching the behavior of getProcessesMetadata where
// a single process failure does not abort the whole scan.
func sumOpenFDs(ctx context.Context, procs []procFD) (int64, error) {
	var total int64
	for _, p := range procs {
		n, err := p.NumFDsWithContext(ctx)
		if err != nil {
			continue
		}
		total += int64(n)
	}
	return total, nil
}
