package remotewrite

import (
	"github.com/prometheus/prometheus/prompb"
	"go.k6.io/k6/metrics"
)

func tagsToLabels(tags *metrics.SampleTags, config Config) ([]prompb.Label, error) {
	if !config.KeepTags.Bool {
		return []prompb.Label{}, nil
	}

	tagsMap := tags.CloneTags()

	hostIndicator := 0
	if config.WriteMetrics["host"] {
		hostIndicator = 1
	}

	labelPairs := make([]prompb.Label, 0, len(tagsMap)+hostIndicator)

	for name, value := range tagsMap {
		if len(name) < 1 || len(value) < 1 {
			continue
		}

		if !config.KeepNameTag.Bool && name == "name" {
			continue
		}

		if !config.KeepUrlTag.Bool && name == "url" {
			continue
		}

		labelPairs = append(labelPairs, prompb.Label{
			Name:  name,
			Value: value,
		})
	}
	if hostIndicator == 1 {
		labelPairs = append(labelPairs, prompb.Label{
			Name:  "host",
			Value: hostName,
		})
	}

	// names of the metrics might be remote agent dependent so let Mapping set those

	return labelPairs[:len(labelPairs):len(labelPairs)], nil
}
