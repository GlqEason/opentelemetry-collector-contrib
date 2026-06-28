// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin || freebsd || openbsd || windows

package processesscraper

import (
	"context"
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/load"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/scraper/scrapererror"
	"go.opentelemetry.io/collector/scraper/scrapertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/hostmetricsreceiver/internal/scraper/processesscraper/internal/metadata"
)

// fakeProcFD is a test double for procFD that returns a canned open file
// descriptor count (or an error) regardless of the context passed in.
type fakeProcFD struct {
	n   int32
	err error
}

func (f fakeProcFD) NumFDsWithContext(_ context.Context) (int32, error) {
	return f.n, f.err
}

func TestSumOpenFDs(t *testing.T) {
	tests := []struct {
		name    string
		procs   []procFD
		want    int64
		wantErr bool
	}{
		{
			name:  "accumulates across processes",
			procs: []procFD{fakeProcFD{n: 3}, fakeProcFD{n: 5}, fakeProcFD{n: 2}},
			want:  10,
		},
		{
			name:  "skips processes that return an error",
			procs: []procFD{fakeProcFD{n: 4}, fakeProcFD{err: errors.New("permission denied")}, fakeProcFD{n: 6}},
			want:  10,
		},
		{
			name:  "empty process list yields zero",
			procs: []procFD{},
			want:  0,
		},
		{
			name:  "all processes error yields zero",
			procs: []procFD{fakeProcFD{err: errors.New("e1")}, fakeProcFD{err: errors.New("e2")}},
			want:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sumOpenFDs(t.Context(), tc.procs)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestScrapeOpenFileDescriptors(t *testing.T) {
	ctx := t.Context()

	t.Run("emits metric when enabled", func(t *testing.T) {
		s := newProcessesScraper(ctx, scrapertest.NewNopSettings(metadata.Type), &Config{
			MetricsBuilderConfig: enableOpenFDMetricsConfig(),
		})
		require.NoError(t, s.start(ctx, componenttest.NewNopHost()))

		// Suppress the unrelated count/created data sources so the scrape focuses on fd.
		s.getMiscStats = func(context.Context) (*load.MiscStat, error) { return &fakeData, nil }
		s.getProcesses = func(context.Context) ([]proc, error) { return fakeProcessesData, nil }
		s.getOpenFDs = func(context.Context) (int64, error) { return 42, nil }

		md, err := s.scrape(ctx)
		require.NoError(t, err)

		metric := findMetric(t, md, "system.processes.open_file_descriptors")
		require.Equal(t, pmetric.MetricTypeSum, metric.Type())
		require.False(t, metric.Sum().IsMonotonic())

		dps := metric.Sum().DataPoints()
		require.Equal(t, 1, dps.Len())
		assert.Equal(t, int64(42), dps.At(0).IntValue())
	})

	t.Run("does not emit metric when disabled", func(t *testing.T) {
		s := newProcessesScraper(ctx, scrapertest.NewNopSettings(metadata.Type), &Config{
			MetricsBuilderConfig: metadata.NewDefaultMetricsBuilderConfig(),
		})
		require.NoError(t, s.start(ctx, componenttest.NewNopHost()))

		s.getMiscStats = func(context.Context) (*load.MiscStat, error) { return &fakeData, nil }
		s.getProcesses = func(context.Context) ([]proc, error) { return fakeProcessesData, nil }
		s.getOpenFDs = func(context.Context) (int64, error) { return 42, nil }

		md, err := s.scrape(ctx)
		require.NoError(t, err)

		for i := 0; i < md.MetricCount(); i++ {
			assert.NotEqual(t, "system.processes.open_file_descriptors", md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(i).Name())
		}
	})

	t.Run("returns partial error on fd failure", func(t *testing.T) {
		s := newProcessesScraper(ctx, scrapertest.NewNopSettings(metadata.Type), &Config{
			MetricsBuilderConfig: enableOpenFDMetricsConfig(),
		})
		require.NoError(t, s.start(ctx, componenttest.NewNopHost()))

		s.getMiscStats = func(context.Context) (*load.MiscStat, error) { return &fakeData, nil }
		s.getProcesses = func(context.Context) ([]proc, error) { return fakeProcessesData, nil }
		s.getOpenFDs = func(context.Context) (int64, error) { return 0, errors.New("enumerate failed") }

		_, err := s.scrape(ctx)
		require.Error(t, err)

		var partial scrapererror.PartialScrapeError
		require.ErrorAs(t, err, &partial)
		assert.Equal(t, 1, partial.Failed)
	})
}

// enableOpenFDMetricsConfig returns a builder config with the
// system.processes.open_file_descriptors metric enabled (it is disabled by
// default) so tests can exercise the new code path.
func enableOpenFDMetricsConfig() metadata.MetricsBuilderConfig {
	cfg := metadata.NewDefaultMetricsBuilderConfig()
	cfg.Metrics.SystemProcessesOpenFileDescriptors.Enabled = true
	return cfg
}

func findMetric(t *testing.T, md pmetric.Metrics, name string) pmetric.Metric {
	t.Helper()
	metrics := md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics()
	for i := 0; i < metrics.Len(); i++ {
		if metrics.At(i).Name() == name {
			return metrics.At(i)
		}
	}
	require.Failf(t, "metric not found", "expected metric %q in scrape output", name)
	return pmetric.Metric{}
}
