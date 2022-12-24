package remotewrite

import (
	"github.com/nano-interactive/PerformanceTest-xk6PrometheusWrite/pkg/remotewrite"
	"go.k6.io/k6/output"
)

func init() {
	output.RegisterExtension("output-nano", func(p output.Params) (output.Output, error) {
		return remotewrite.New(p)
	})
}
