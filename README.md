# ambient-wx-exporter

A [prometheus](https://prometheus.io) metrics exporter for [Ambient
Weather](https://ambientweather.net) personal weather station data.  The basic
idea is that this application will fetch data from the Ambient API
([docs](https://ambientweather.docs.apiary.io/)) and repackage it into
Prometheus metrics format and serve it over HTTP as a Prometheus scraping
target.

This is very much work in progress.

```
go build
./ambient-wx-exporter $APP_KEY $API_KEY --port 9999
```

Then you should see metrics at `http://localhost:9999/metrics`.
