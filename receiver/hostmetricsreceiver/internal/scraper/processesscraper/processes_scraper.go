// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package processesscraper // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processesscraper"

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/process"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/scraper"
	"go.opentelemetry.io/collector/scraper/scrapererror"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processesscraper/internal/metadata"
)

var metricsLength = func() int {
	n := 0
	if enableProcessesCount {
		n++
	}
	if enableProcessesCreated {
		n++
	}
	if enableOpenFileDescriptors {
		n++
	}
	return n
}()

// countCreatedMetricsLen is the number of metrics produced by
// getProcessesMetadata (system.processes.count and system.processes.created).
// It is reported as the failed count when that metadata collection fails, so
// the open file descriptors metric is not counted as failed by a failure in
// the unrelated count/created data source (and vice versa).
var countCreatedMetricsLen = func() int {
	n := 0
	if enableProcessesCount {
		n++
	}
	if enableProcessesCreated {
		n++
	}
	return n
}()

// scraper for Processes Metrics
type processesScraper struct {
	settings scraper.Settings
	config   *Config
	mb       *metadata.MetricsBuilder

	// for mocking gopsutil
	getMiscStats func(context.Context) (*load.MiscStat, error)
	getProcesses func(context.Context) ([]proc, error)
	getOpenFDs   func(context.Context) (int64, error)
	bootTime     func(context.Context) (uint64, error)
}

// for mocking out gopsutil process.Process
type proc interface {
	Status() ([]string, error)
}

type processesMetadata struct {
	countByStatus    map[metadata.AttributeStatus]int64 // ignored if enableProcessesCount is false
	processesCreated *int64                             // ignored if enableProcessesCreated is false
}

// newProcessesScraper creates a set of Processes related metrics
func newProcessesScraper(_ context.Context, settings scraper.Settings, cfg *Config) *processesScraper {
	return &processesScraper{
		settings:     settings,
		config:       cfg,
		getMiscStats: load.MiscWithContext,
		getProcesses: func(ctx context.Context) ([]proc, error) {
			ps, err := process.ProcessesWithContext(ctx)
			ret := make([]proc, len(ps))
			for i := range ps {
				ret[i] = ps[i]
			}
			return ret, err
		},
		getOpenFDs: getProcessOpenFDs,
		bootTime:   host.BootTimeWithContext,
	}
}

func (s *processesScraper) start(ctx context.Context, _ component.Host) error {
	bootTime, err := s.bootTime(ctx)
	if err != nil {
		return err
	}

	s.mb = metadata.NewMetricsBuilder(s.config.MetricsBuilderConfig, s.settings, metadata.WithStartTime(pcommon.Timestamp(bootTime*1e9)))
	return nil
}

func (s *processesScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	now := pcommon.NewTimestampFromTime(time.Now())

	md := pmetric.NewMetrics()
	metrics := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics()
	metrics.EnsureCapacity(metricsLength)

	var errs scrapererror.ScrapeErrors

	processMetadata, err := s.getProcessesMetadata(ctx)
	if err != nil {
		errs.AddPartial(countCreatedMetricsLen, err)
	} else {
		if enableProcessesCount && processMetadata.countByStatus != nil {
			for status, count := range processMetadata.countByStatus {
				s.mb.RecordSystemProcessesCountDataPoint(now, count, status)
			}
		}

		if enableProcessesCreated && processMetadata.processesCreated != nil {
			s.mb.RecordSystemProcessesCreatedDataPoint(now, *processMetadata.processesCreated)
		}
	}

	if enableOpenFileDescriptors && s.config.Metrics.SystemProcessesOpenFileDescriptors.Enabled {
		fds, fdErr := s.getOpenFDs(ctx)
		if fdErr != nil {
			errs.AddPartial(1, fdErr)
		} else {
			s.mb.RecordSystemProcessesOpenFileDescriptorsDataPoint(now, fds)
		}
	}

	return s.mb.Emit(), errs.Combine()
}
