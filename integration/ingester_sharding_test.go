// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/integration/ingester_sharding_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.
//go:build requires_docker
// +build requires_docker

package integration

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/grafana/e2e"
	e2edb "github.com/grafana/e2e/db"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/mimir/integration/e2emimir"
)

func TestIngesterSharding(t *testing.T) {
	const numSeriesToPush = 1000

	tests := map[string]struct {
		tenantShardSize             int
		expectedIngestersWithSeries int
	}{
		"zero shard size should spread series across all ingesters": {
			tenantShardSize:             0,
			expectedIngestersWithSeries: 3,
		},
		"non-zero shard size should spread series across the configured shard size number of ingesters": {
			tenantShardSize:             2,
			expectedIngestersWithSeries: 2,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			s, err := e2e.NewScenario(networkName)
			require.NoError(t, err)
			defer s.Close()

			flags := BlocksStorageFlags()
			flags["-distributor.ingestion-tenant-shard-size"] = strconv.Itoa(testData.tenantShardSize)

			// Start dependencies.
			consul := e2edb.NewConsul()
			minio := e2edb.NewMinio(9000, flags["-blocks-storage.s3.bucket-name"])
			require.NoError(t, s.StartAndWaitReady(consul, minio))

			// Start Mimir components.
			distributor := e2emimir.NewDistributor("distributor", consul.NetworkHTTPEndpoint(), flags)
			ingester1 := e2emimir.NewIngester("ingester-1", consul.NetworkHTTPEndpoint(), flags)
			ingester2 := e2emimir.NewIngester("ingester-2", consul.NetworkHTTPEndpoint(), flags)
			ingester3 := e2emimir.NewIngester("ingester-3", consul.NetworkHTTPEndpoint(), flags)
			querier := e2emimir.NewQuerier("querier", consul.NetworkHTTPEndpoint(), flags)
			require.NoError(t, s.StartAndWaitReady(distributor, ingester1, ingester2, ingester3, querier))

			// Wait until distributor and queriers have updated the ring.
			require.NoError(t, distributor.WaitSumMetricsWithOptions(e2e.Equals(3), []string{"cortex_ring_members"}, e2e.WithLabelMatchers(
				labels.MustNewMatcher(labels.MatchEqual, "name", "ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "state", "ACTIVE"))))

			require.NoError(t, querier.WaitSumMetricsWithOptions(e2e.Equals(3), []string{"cortex_ring_members"}, e2e.WithLabelMatchers(
				labels.MustNewMatcher(labels.MatchEqual, "name", "ingester"),
				labels.MustNewMatcher(labels.MatchEqual, "state", "ACTIVE"))))

			// Push series.
			now := time.Now()
			expectedVectors := map[string]model.Vector{}

			client, err := e2emimir.NewClient(distributor.HTTPEndpoint(), querier.HTTPEndpoint(), "", "", userID)
			require.NoError(t, err)

			for i := 1; i <= numSeriesToPush; i++ {
				metricName := fmt.Sprintf("series_%d", i)
				series, expectedVector := generateSeries(metricName, now)
				res, err := client.Push(series)
				require.NoError(t, err)
				require.Equal(t, 200, res.StatusCode)

				expectedVectors[metricName] = expectedVector
			}

			// Extract metrics from ingesters.
			numIngestersWithSeries := 0
			totalIngestedSeries := 0

			for _, ing := range []*e2emimir.MimirService{ingester1, ingester2, ingester3} {
				values, err := ing.SumMetrics([]string{"cortex_ingester_memory_series"})
				require.NoError(t, err)

				numMemorySeries := e2e.SumValues(values)
				totalIngestedSeries += int(numMemorySeries)
				if numMemorySeries > 0 {
					numIngestersWithSeries++
				}
			}

			// Verify that the expected number of ingesters had series (write path). However,
			// we _don't_ verify that a subset of ingesters were queried for the series (read
			// path). This is because the way which ingesters to query is calculated depends on
			// when they were registered compared to the "query ingesters within" time. They'll
			// never be registered long enough as part of this test to be queried for the series.
			// Instead, all ingesters are queried.
			require.Equal(t, testData.expectedIngestersWithSeries, numIngestersWithSeries)
			require.Equal(t, numSeriesToPush, totalIngestedSeries)

			// Query back series.
			for metricName, expectedVector := range expectedVectors {
				result, err := client.Query(metricName, now)
				require.NoError(t, err)
				require.Equal(t, model.ValVector, result.Type())
				assert.Equal(t, expectedVector, result.(model.Vector))
			}

			// Ensure no service-specific metrics prefix is used by the wrong service.
			assertServiceMetricsPrefixes(t, Distributor, distributor)
			assertServiceMetricsPrefixes(t, Ingester, ingester1)
			assertServiceMetricsPrefixes(t, Ingester, ingester2)
			assertServiceMetricsPrefixes(t, Ingester, ingester3)
			assertServiceMetricsPrefixes(t, Querier, querier)
		})
	}
}
