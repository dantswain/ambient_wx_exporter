package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/alecthomas/kong"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/dantswain/ambient_wx_exporter/pkg/ambient_wx_exporter/config"
	"github.com/dantswain/ambient_wx_exporter/pkg/ambient_wx_exporter/metrics"
	"github.com/dantswain/ambient_wx_exporter/pkg/ambient_wx_exporter/state"
)

var cli struct {
	AppKey               string `arg:"true" help:"Ambient APP key"`
	APIKey               string `arg:"true" help:"Ambient API key"`
	Port                 uint16 `default:"9876" help:"http port to listen on"`
	ConfigFile           string `help:"Path to json config file"`
	MetricPrefix         string `help:"Metric name prefix" default:"ambient_wx_"`
	Debug                bool   `help:"Set log to debug"`
	DisableDefaultGauges bool   `help:"Disable default gauge metrics"`
}

func main() {
	kong.Parse(&cli)

	logger := log.NewLogfmtLogger(os.Stderr)
	logLevel := level.AllowInfo()
	if cli.Debug {
		logLevel = level.AllowDebug()
	}
	logger = level.NewFilter(logger, logLevel)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	theConfig := &config.Config{}
	if cli.ConfigFile != "" {
		err := config.Read(cli.ConfigFile, theConfig)
		if err != nil {
			level.Error(logger).Log("msg", err)
			os.Exit(1)
		}
	}

	theState := state.Init(cli.MetricPrefix, cli.AppKey, cli.APIKey, theConfig, cli.DisableDefaultGauges)

	metrics.RecordMetrics(theState, logger)

	listen := fmt.Sprintf(":%d", cli.Port)
	http.Handle("/metrics", promhttp.Handler())

	level.Info(logger).Log("msg", fmt.Sprintf("Metrics will be served on http://localhost:%d", cli.Port))
	level.Error(logger).Log(http.ListenAndServe(listen, nil))
}
