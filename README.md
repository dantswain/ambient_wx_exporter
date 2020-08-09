# ambient-wx-exporter

[![Build Status](https://travis-ci.org/dantswain/ambient_wx_exporter.svg?branch=master)](https://travis-ci.org/dantswain/ambient_wx_exporter)

A [prometheus](https://prometheus.io) metrics exporter for [Ambient
Weather](https://ambientweather.net) personal weather station data.  The basic
idea is that this application will fetch data from the Ambient API
([docs](https://ambientweather.docs.apiary.io/)) and repackage it into
Prometheus metrics format and serve it over HTTP as a Prometheus scraping
target.

This is very much work in progress.

## Usage

First, build the project:

```
make build
```

You'll need your app and API keys from Ambient, which you can get from your
profile on <ambientweather.net>.  Below I will reference them as environment
variables `$APP_KEY` and `$API_KEY`, respectively.

To use the default set of metrics, which is essentially just a pass-through of
the API return values labeled by MAC address, you can run the code directly:

```
./ambient-wx-exporter $APP_KEY $API_KEY
```

Then you should see metrics at `http://localhost:9876/metrics`.

If you want finer-grained control over how metrics are reported (e.g., group
all temperatues as "temperature" and label by sensor location), modify
`sample_config.json` to suit your needs. Most importantly, modify the
`mac_address` to match the MAC of your device. The metric names and tagging
are merely suggestions, there is nothing special about them (it's based on
what I use).  You may need to disable the default gauges if any of your metric
names collide with the defaults (the sample conflicts with `humidity`).

```
./ambient-wx-exporter $APP_KEY $API_KEY --config-file sample_config.json --disable-default-gauges
```

```
Usage: exporter <app-key> <api-key>

Arguments:
  <app-key>    Ambient APP key
  <api-key>    Ambient API key

Flags:
  -h, --help                      Show context-sensitive help.
      --port=9876                 http port to listen on
      --config-file=STRING        Path to json config file
      --debug                     Set log to debug
      --disable-default-gauges    Disable default gauge metrics
```
