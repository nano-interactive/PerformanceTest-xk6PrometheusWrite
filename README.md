# xk6-output-nano
k6 extension for publishing test-run metrics to Prometheus via Remote Write endpoint with the support of Histogram latency response.

There are many options for remote-write compatible agents, the official list can be found [here](https://prometheus.io/docs/operating/integrations/). The exact details of how metrics will be processed or stored depends on the underlying agent used.

Key points to know:

- remote write format does not contain explicit definition of any metric types while metadata definition is still in flux and can have different implementation depending on the remote-write compatible agent
- remote read is a separate interface and it is much less defined. For example, remote read may not work without precise queries; see [here](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) and [here](https://github.com/timescale/promscale/issues/64) for details
- some remote-write compatible agents may support additional formats for remote write, like JSON, but it is not part of official Prometheus remote write specification and therefore absent here

### Usage
Install xk6. Version `0.7.0` is tested, you can try with `@latest`
``` 
go install go.k6.io/xk6/cmd/xk6@v0.7.0
```

To build k6 binary with the Prometheus remote write output extension use:
```
xk6 build --with github.com/radepopovic/xk6-output-nano@latest 
```

Then run new k6 binary with:
```
K6_PROMETHEUS_REMOTE_URL=http://localhost:9090/api/v1/write ./k6 run example/test.js -o output-nano
```

Add TLS and HTTP basic authentication:
```
K6_PROMETHEUS_REMOTE_URL=https://localhost:9090/api/v1/write K6_PROMETHEUS_INSECURE_SKIP_TLS_VERIFY=false K6_CA_CERT_FILE=example/tls.crt K6_PROMETHEUS_USER=foo K6_PROMETHEUS_PASSWORD=bar ./k6 run example/test.js -o output-nano
```

Different remote storage agents are supported with mapping option. The default is Prometheus itself but there is a simpler raw mapping that can be used as a starting point for other remote agents:
```
K6_PROMETHEUS_MAPPING=raw K6_PROMETHEUS_REMOTE_URL=http://localhost:9090/api/v1/write ./k6 run example/test.js -o output-nano
```

Note: Prometheus remote client relies on a snappy library for serialization which can panic on [encode operation](https://github.com/golang/snappy/blob/544b4180ac705b7605231d4a4550a1acb22a19fe/encode.go#L22).

## Make file
We provided `Makefile` for the convenience. You can build `k6` with the local source code with
``` 
make
```
Run tests
``` 
make test
```


### Remote Metrics

The extension are not sending builtin metrics by defaul and it will only send 
nano custom k6 metrics.
If you would like to send builtin k6 metrics set parameter `K6_NANO_WRITE_METRICS`
```
K6_NANO_WRITE_METRICS=http_req_duration,data_sent,data_received,vus,vus_max K6_PROMETHEUS_REMOTE_URL=http://localhost:9090/api/v1/write ./k6 run example/test.js -o output-nano
```

`K6_KEEP_URL_TAG` is disabled by default.

It will also create additional `k6_nano_duration_bucket`, `k6_nano_duration_count` and `k6_nano_duration_sum` Prometheus Histogram metric 
that sends response latency time in buckets.

Bucket distribution can be set by `K6_NANO_HISTOGRAM_BUCKETS` parameter
```
K6_NANO_HISTOGRAM_BUCKETS=0.5,1,2,3,4,5,6,7,8,9,10,12,15,20,30,40,50,70,100,500,1000 K6_PROMETHEUS_REMOTE_URL=http://localhost:9090/api/v1/write ./k6 run example/test.js -o output-nano
```

### On sample rate
k6 processes its outputs once per second and that is also a default flush period in this extension. The number of k6 builtin metrics is 26 and they are collected at the rate of 50ms. In practice it means that there will be around 1000-1500 samples on average per each flush period in case of raw mapping. If custom metrics are configured, that estimate will have to be adjusted.

Depending on exact setup, it may be necessary to configure Prometheus and / or remote-write agent to handle the load. For example, see [`queue_config` parameter](https://prometheus.io/docs/practices/remote_write/) of Prometheus.

If remote endpoint responds too slowly or the k6 test run generates too many metrics, extension may start discarding samples in order to continue to adhere to the flush period.

### Prometheus as remote-write agent

To enable remote write in Prometheus 2.x use `--enable-feature=remote-write-receiver` option. See docker-compose samples in `example/`. Options for remote write storage can be found [here](https://prometheus.io/docs/operating/integrations/). 


# Docker Compose

This repo includes a [docker-compose.yml](docker-compose.yml) file that starts _Prometheus_, _Grafana_.

Clone the repo to get started and follow these steps: 

1. Start the docker compose environment.
    ```shell
    docker-compose up -d
    ```

    ```shell
    # Output
    Creating xk6-output-prometheus-remote_grafana_1     ... done
    Creating xk6-output-prometheus-remote_prometheus_1  ... done
    ```

2. Visit http://localhost:3000/ to view results in Grafana.
