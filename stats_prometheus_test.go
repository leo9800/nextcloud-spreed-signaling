/**
 * Standalone signaling server for the Nextcloud Spreed app.
 * Copyright (C) 2021 struktur AG
 *
 * @author Joachim Bauch <bauch@struktur.de>
 *
 * @license GNU AGPL version 3 or any later version
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package signaling

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func ResetStatsValue[T prometheus.Gauge](t *testing.T, collector T) {
	// Make sure test is not executed with "t.Parallel()"
	t.Setenv("PARALLEL_CHECK", "1")

	collector.Set(0)
	t.Cleanup(func() {
		collector.Set(0)
	})
}

func checkStatsValue(t *testing.T, collector prometheus.Collector, value float64) {
	// Make sure test is not executed with "t.Parallel()"
	t.Setenv("PARALLEL_CHECK", "1")

	ch := make(chan *prometheus.Desc, 1)
	collector.Describe(ch)
	desc := <-ch
	v := testutil.ToFloat64(collector)
	if v != value {
		assert := assert.New(t)
		pc := make([]uintptr, 10)
		n := runtime.Callers(2, pc)
		if n == 0 {
			assert.EqualValues(value, v, "failed for %s", desc)
			return
		}

		pc = pc[:n]
		frames := runtime.CallersFrames(pc)
		stack := ""
		for {
			frame, more := frames.Next()
			if !strings.Contains(frame.File, "nextcloud-spreed-signaling") {
				break
			}
			stack += fmt.Sprintf("%s:%d\n", frame.File, frame.Line)
			if !more {
				break
			}
		}
		assert.EqualValues(value, v, "Unexpected value for %s at\n%s", desc, stack)
	}
}

func collectAndLint(t *testing.T, collectors ...prometheus.Collector) {
	assert := assert.New(t)
	for _, collector := range collectors {
		problems, err := testutil.CollectAndLint(collector)
		if !assert.NoError(err) {
			continue
		}

		for _, problem := range problems {
			assert.Fail("Problem with metric", "%s: %s", problem.Metric, problem.Text)
		}
	}
}
