package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"net/http"

	"github.com/alecthomas/kong"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"github.com/lrosenman/ambient"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var apiKeys = [...]string{
	"baromabsin", "baromrelin",
	"batt1", "batt2", "batt3", "batt4", "batt5", "batt6", "batt7", "batt8", "battin", "battout",
	"dewPoint", "dewPoint1", "dewPoint2", "dewPoint3", "dewPoint4", "dewPoint5",
	"dewPoint6", "dewPoint7", "dewPoint8", "dewPointin",
	"dailyrainin", "eventrainin", "hourlyrainin",
	"feelsLike", "feelsLike1", "feelsLike2", "feelsLike3", "feelsLike4", "feelsLike5",
	"feelsLike6", "feelsLike7", "feelsLike8", "feelsLikein",
	"humidity", "humidity1", "humidity2", "humidity3", "humidity4", "humidity5",
	"humidity6", "humidity7", "humidity8", "humidityin",
	"lastRain", "maxdailygust", "monthlyrainin", "solarradiation",
	"temp1f", "temp2f", "temp3f", "temp4f", "temp5f", "temp6f", "temp7f", "temp8f",
	"tempf", "tempinf", "uv", "weeklyrainin",
	"winddir", "winddir_avg10m", "windgustmph", "windspdmph_avg10m",
	"windspeedmph", "yearlyrainin",
}

var apiKeysWithoutMetrics = map[string]bool{
	"dateutc":  true,
	"date":     true,
	"tz":       true,
	"lastRain": true,
}

var logger log.Logger

var cli struct {
	AppKey               string `arg:"true" help:"Ambient APP key"`
	APIKey               string `arg:"true" help:"Ambient API key"`
	Port                 uint16 `default:"9876" help:"http port to listen on"`
	ConfigFile           string `help:"Path to json config file"`
	Debug                bool   `help:"Set log to debug"`
	DisableDefaultGauges bool   `help:"Disable default gauge metrics"`
}

type gaugeConfig struct {
	APIName string            `json:"api_name"`
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels"`
}

type deviceConfig struct {
	MacAddress string `json:"mac_address"`
	Gauges     []gaugeConfig
}

type gaugeDict map[string](*prometheus.GaugeVec)
type stringDict map[string]string
type stringSet map[string](struct{})

var config struct {
	Devices []deviceConfig
}

var state struct {
	GaugesByName  gaugeDict
	LabelsByName  map[string]stringSet
	Gauges        map[string]gaugeDict
	Labels        map[string](map[string](stringDict))
	DefaultGauges gaugeDict
}

func getAmbientDevices(key ambient.Key) ambient.APIDeviceResponse {
	dr, err := ambient.Device(key)
	if err != nil {
		panic(err)
	}
	switch dr.HTTPResponseCode {
	case 200:
	case 429, 502, 503:
		{
			level.Warn(logger).Log(
				"msg", "HTTP error from Ambient API. Retrying.",
				"status_code", dr.HTTPResponseCode,
			)
			time.Sleep(1 * time.Second)
			dr, err = ambient.Device(key)
			if err != nil {
				panic(err)
			}
			switch dr.HTTPResponseCode {
			case 200:
			default:
				{
					panic(dr)
				}
			}
		}
	default:
		{
			level.Error(logger).Log(
				"msg", "Unrecoverable HTTP error from Ambient API.",
				"status_code", dr.HTTPResponseCode,
			)
			panic(dr)
		}
	}

	level.Info(logger).Log("msg", "Succesfully fetched data from Ambient API")
	return dr
}

func makeGaugeVec(name string) *prometheus.GaugeVec {
	return promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: fmt.Sprintf("Value of %s reported by Ambient API", name),
	}, []string{"device_mac"})
}

func setGauge(gauge *prometheus.GaugeVec, mac string, val float64) {
	gauge.With(prometheus.Labels{"device_mac": mac}).Set(val)
}

func setBattGauge(gauge *prometheus.GaugeVec, mac string, val json.Number) {
	if val.String() == "" {
		return
	}

	f, err := val.Float64()
	if err != nil {
		panic(err)
	}
	setGauge(gauge, mac, f)
}

func setGaugeInterface(gauge *prometheus.GaugeVec, labels map[string]string, val interface{}) {
	switch v := val.(type) {
	// case string:
	// 	gauge.With(labels).Set(0.0)
	case int64:
		gauge.With(labels).Set(float64(v))
	case float64:
		gauge.With(labels).Set(v)
	default:
		level.Warn(logger).Log("msg", "Unhandled metric type", "type", fmt.Sprintf("%T", v))
	}
}

type gaugeMetric struct {
	Gauge  *prometheus.GaugeVec
	Labels map[string]string
}

type device struct {
	Gauges map[string]gaugeMetric
}

func makeDevice(mac string) device {
	gauges := make(map[string]gaugeMetric)

	return device{Gauges: gauges}
}

func recordDeviceMetrics(device ambient.DeviceRecord) {
	for k, v := range device.LastDataFields {
		gauges, ok := state.Gauges[device.Macaddress]
		if ok {
			gauge, ok := gauges[k]
			if ok {
				labels := state.Labels[device.Macaddress][k]
				setGaugeInterface(gauge, labels, v)
			} else {
				level.Debug(logger).Log("msg", "No config for ambient metric", "mac_address", device.Macaddress, "api_key", k)
			}
		} else {
			level.Warn(logger).Log("msg", "No config for mac address", "mac_address", device.Macaddress)
		}
	}
}

func recordDefaultMetrics(device ambient.DeviceRecord) {
	labels := map[string]string{"mac_address": device.Macaddress}
	for k, v := range device.LastDataFields {
		if apiKeysWithoutMetrics[k] {
			continue
		}
		gauge, ok := state.DefaultGauges[k]
		if ok {
			setGaugeInterface(gauge, labels, v)
		} else {
			level.Warn(logger).Log("msg", "No default metric defined for api key", "mac_address", device.Macaddress, "api_key", k)
		}
	}
}

func recordMetrics(key ambient.Key) {
	go func() {
		for {
			dr := getAmbientDevices(key)

			for _, device := range dr.DeviceRecord {
				level.Info(logger).Log("msg", "Recording device metrics", "mac_address", device.Macaddress)
				recordDeviceMetrics(device)
				if !cli.DisableDefaultGauges {
					recordDefaultMetrics(device)
				}

				time.Sleep(60 * time.Second)
			}
		}
	}()
}

func populateState() {
	state.GaugesByName = make(gaugeDict)
	state.LabelsByName = make(map[string](stringSet))
	state.Gauges = make(map[string](gaugeDict))
	state.Labels = make(map[string](map[string]stringDict))
	state.DefaultGauges = make(gaugeDict)

	for _, d := range config.Devices {
		for _, g := range d.Gauges {
			_, ok := state.LabelsByName[g.Name]
			if !ok {
				state.LabelsByName[g.Name] = make(map[string](struct{}))
			}
			for k := range g.Labels {
				state.LabelsByName[g.Name][k] = struct{}{}
			}
			state.LabelsByName[g.Name]["mac_address"] = struct{}{}
		}
	}

	for n, l := range state.LabelsByName {
		labels := make([]string, len(l))
		ix := 0
		for ll := range l {
			labels[ix] = ll
			ix++
		}

		state.GaugesByName[n] = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: n,
		}, labels)
	}

	for _, d := range config.Devices {
		state.Gauges[d.MacAddress] = make(map[string](*prometheus.GaugeVec))
		state.Labels[d.MacAddress] = make(map[string]stringDict)
		for _, g := range d.Gauges {
			state.Gauges[d.MacAddress][g.APIName] = state.GaugesByName[g.Name]
			labels := g.Labels
			labels["mac_address"] = d.MacAddress
			state.Labels[d.MacAddress][g.APIName] = labels
		}
	}

	if !cli.DisableDefaultGauges {
		for _, n := range apiKeys {
			state.DefaultGauges[n] = promauto.NewGaugeVec(prometheus.GaugeOpts{
				Name: n,
			}, []string{"mac_address"})
		}
	}
}

func main() {
	kong.Parse(&cli)

	logger = log.NewLogfmtLogger(os.Stderr)
	levels := []level.Option{level.AllowInfo(), level.AllowError(), level.AllowWarn()}
	if cli.Debug {
		levels = append(levels, level.AllowDebug())
	}
	logger = level.NewFilter(logger, levels...)
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	if cli.ConfigFile != "" {
		file, err := os.Open(cli.ConfigFile)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		bytes, _ := ioutil.ReadAll(file)
		json.Unmarshal(bytes, &config)
		populateState()
	}

	key := ambient.NewKey(cli.AppKey, cli.APIKey)

	recordMetrics(key)

	listen := fmt.Sprintf(":%d", cli.Port)
	http.Handle("/metrics", promhttp.Handler())

	level.Info(logger).Log("msg", fmt.Sprintf("Metrics will be served on http://localhost:%d", cli.Port))
	level.Error(logger).Log(http.ListenAndServe(listen, nil))
}
