package remotewrite

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"

	"os"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/sirupsen/logrus"
	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"
)

type Output struct {
	config Config

	client          remote.WriteClient
	metrics         *metricsStorage
	mapping         Mapping
	periodicFlusher *output.PeriodicFlusher
	output.SampleBuffer

	logger logrus.FieldLogger
}

var _ output.Output = new(Output)

// toggle to indicate whether we should stop dropping samples
var flushTooLong bool
var defaultHistogramBuckets = []float64{0.5, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 15, 20, 30, 40, 50, 70, 100, 500, 1000}

var hostName string

// RP change
var prmtHistogram *prometheus.HistogramVec
var reg *prometheus.Registry

func New(params output.Params) (*Output, error) {
	config, err := GetConsolidatedConfig(params.JSONConfig, params.Environment, params.ConfigArgument)
	if err != nil {
		return nil, err
	}

	prmtHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "k6_nano_duration",
		Help:    "Request latency distributions",
		Buckets: config.HistogramBuckets,
	}, []string{"status", "host"})
	reg = prometheus.NewRegistry()
	reg.MustRegister(prmtHistogram)

	hostName, err = os.Hostname()
	if err != nil {
		hostName = "host_rnd_" + strconv.Itoa(int(time.Now().UnixMilli()))
	}

	remoteConfig, err := config.ConstructRemoteConfig()
	if err != nil {
		return nil, err
	}

	// name is used to differentiate clients in metrics
	client, err := remote.NewWriteClient("xk6-prwo", remoteConfig)
	if err != nil {
		return nil, err
	}

	params.Logger.Info(fmt.Sprintf("Prometheus: configuring remote-write with %s mapping", config.Mapping.String))

	return &Output{
		client:  client,
		config:  config,
		metrics: newMetricsStorage(),
		mapping: NewMapping(config.Mapping.String),
		logger:  params.Logger,
	}, nil
}

func (*Output) Description() string {
	return "Output k6 metrics to prometheus remote-write endpoint and creates Prometheus Histogram metric for latency"
}

func (o *Output) Start() error {
	if periodicFlusher, err := output.NewPeriodicFlusher(time.Duration(o.config.FlushPeriod.Duration), o.flush); err != nil {
		return err
	} else {
		o.periodicFlusher = periodicFlusher
	}
	o.logger.Debug("Prometheus: starting remote-write")

	return nil
}

func (o *Output) Stop() error {
	o.logger.Debug("Prometheus: stopping remote-write")
	o.periodicFlusher.Stop()
	return nil
}

func (o *Output) flush() {
	var (
		start = time.Now()
		nts   int
	)

	defer func() {
		d := time.Since(start)
		if d > time.Duration(o.config.FlushPeriod.Duration) {
			// There is no intermediary storage so warn if writing to remote write endpoint becomes too slow
			o.logger.WithField("nts", nts).
				Warn(fmt.Sprintf("Remote write took %s while flush period is %s. Some samples may be dropped.",
					d.String(), o.config.FlushPeriod.String()))
			flushTooLong = true
		} else {
			o.logger.WithField("nts", nts).Debug(fmt.Sprintf("Remote write took %s.", d.String()))
			flushTooLong = false
		}
	}()

	samplesContainers := o.GetBufferedSamples()
	for _, sampleContainer := range samplesContainers {
		for _, sample := range sampleContainer.GetSamples() {

			if sample.Metric.Name == "http_req_duration" {
				statusValue, _ := sample.Tags.Get("status")
				prmtHistogram.WithLabelValues(statusValue, hostName).Observe(sample.Value)
			}

		}
	}

	// Remote write endpoint accepts TimeSeries structure defined in gRPC. It must:
	// a) contain Labels array
	// b) have a __name__ label: without it, metric might be unquerable or even rejected
	// as a metric without a name. This behaviour depends on underlying storage used.
	// c) not have duplicate timestamps within 1 timeseries, see https://github.com/prometheus/prometheus/issues/9210
	// Prometheus write handler processes only some fields as of now, so here we'll add only them.
	promTimeSeries := o.convertToTimeSeries(samplesContainers)

	promTimeSeries = append(promTimeSeries, o.addLatencyHistogram()...)

	nts = len(promTimeSeries)

	o.logger.WithField("nts", nts).Debug("Converted samples to time series in preparation for sending.")

	req := prompb.WriteRequest{
		Timeseries: promTimeSeries,
	}

	if buf, err := proto.Marshal(&req); err != nil {
		o.logger.WithError(err).Fatal("Failed to marshal timeseries.")
	} else {
		encoded := snappy.Encode(nil, buf) // this call can panic
		if err = o.client.Store(context.Background(), encoded); err != nil {
			o.logger.WithError(err).Error("Failed to store timeseries.")
		}
	}
}

func (o *Output) convertToTimeSeries(samplesContainers []metrics.SampleContainer) []prompb.TimeSeries {
	promTimeSeries := make([]prompb.TimeSeries, 0)

	for _, samplesContainer := range samplesContainers {
		samples := samplesContainer.GetSamples()

		for _, sample := range samples {

			// skip metrics not specified in config
			if !o.config.WriteMetrics[sample.Metric.Name] {
				continue
			}
			// Prometheus remote write treats each label array in TimeSeries as the same
			// for all Samples in those TimeSeries (https://github.com/prometheus/prometheus/blob/03d084f8629477907cab39fc3d314b375eeac010/storage/remote/write_handler.go#L75).
			// But K6 metrics can have different tags per each Sample so in order not to
			// lose info in tags or assign tags wrongly, let's store each Sample in a different TimeSeries, for now.
			// This approach also allows to avoid hard to replicate issues with duplicate timestamps.

			labels, err := tagsToLabels(sample.Tags, o.config)
			if err != nil {
				o.logger.Error(err)
			}

			if newts, err := o.metrics.transform(o.mapping, sample, labels); err != nil {
				o.logger.Error(err)
			} else {
				promTimeSeries = append(promTimeSeries, newts...)
			}
		}

		// Do not blow up if remote endpoint is overloaded and responds too slowly.
		// TODO: consider other approaches
		if flushTooLong && len(promTimeSeries) > 150000 {
			break
		}
	}

	return promTimeSeries
}

func (o *Output) addLatencyHistogram() []prompb.TimeSeries {
	dtoMetricFamily, _ := reg.Gather()
	if len(dtoMetricFamily) == 0 {
		return []prompb.TimeSeries{}
	}

	var ts []prompb.TimeSeries
	for _, metric := range dtoMetricFamily[0].Metric {
		ts = append(ts, o.convertMetricToTimeSeries(dtoMetricFamily[0].GetName(), metric)...)
	}
	return ts
}

func (o *Output) convertMetricToTimeSeries(metricFamilyName string, metric *io_prometheus_client.Metric) []prompb.TimeSeries {

	var ts []prompb.TimeSeries
	var labels []prompb.Label

	timeStamp := time.Now().UnixMilli()

	for _, labelPair := range metric.GetLabel() {
		labels = append(labels, prompb.Label{
			Name:  *labelPair.Name,
			Value: *labelPair.Value,
		})
	}

	for _, bck := range metric.GetHistogram().Bucket {

		ts = append(ts, prompb.TimeSeries{
			Labels: append(labels, []prompb.Label{
				{
					Name:  "__name__",
					Value: fmt.Sprintf("%s%s", metricFamilyName, "_bucket"),
				},
				{
					Name:  "le",
					Value: fmt.Sprintf("%f", *bck.UpperBound),
				},
			}...),
			Samples: []prompb.Sample{
				{
					Value:     float64(*bck.CumulativeCount),
					Timestamp: timeStamp,
				},
			},
		})

	}
	ts = append(ts, prompb.TimeSeries{
		Labels: append(labels, []prompb.Label{
			{
				Name:  "__name__",
				Value: fmt.Sprintf("%s%s", metricFamilyName, "_bucket"),
			},
			{
				Name:  "le",
				Value: "+Inf",
			},
		}...),
		Samples: []prompb.Sample{
			{
				Value:     float64(metric.GetHistogram().GetSampleCount()),
				Timestamp: timeStamp,
			},
		},
	})
	ts = append(ts, prompb.TimeSeries{
		Labels: append(labels, []prompb.Label{
			{
				Name:  "__name__",
				Value: fmt.Sprintf("%s%s", metricFamilyName, "_sum"),
			},
		}...),
		Samples: []prompb.Sample{
			{
				Value:     metric.GetHistogram().GetSampleSum(),
				Timestamp: timeStamp,
			},
		},
	})
	ts = append(ts, prompb.TimeSeries{
		Labels: append(labels, []prompb.Label{
			{
				Name:  "__name__",
				Value: fmt.Sprintf("%s%s", metricFamilyName, "_count"),
			},
		}...),
		Samples: []prompb.Sample{
			{
				Value:     float64(metric.GetHistogram().GetSampleCount()),
				Timestamp: timeStamp,
			},
		},
	})
	return ts
}
