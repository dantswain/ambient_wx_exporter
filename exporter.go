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

var logger log.Logger

var cli struct {
	AppKey     string `arg:"true" help:"Ambient APP key"`
	APIKey     string `arg:"true" help:"Ambient API key"`
	Port       uint16 `default:"9876" help:"http port to listen on"`
	ConfigFile string `help:"Path to json config file"`
	Debug      bool   `help:"Set log to debug"`
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
	GaugesByName gaugeDict
	LabelsByName map[string]stringSet
	Gauges       map[string]gaugeDict
	Labels       map[string](map[string](stringDict))
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
	case string:
		gauge.With(labels).Set(0.0)
	case int64:
		gauge.With(labels).Set(float64(v))
	case float64:
		gauge.With(labels).Set(v)
	default:
		fmt.Println("UNEXPECTED TYPE")
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

func recordMetrics(key ambient.Key) {
	var (
		// float values
		tempInF            = makeGaugeVec("tempinf")
		baromAbsIn         = makeGaugeVec("baromabsin")
		baromRelIn         = makeGaugeVec("baromrelin")
		tempF              = makeGaugeVec("tempf")
		co2                = makeGaugeVec("co2")
		dailyRainIn        = makeGaugeVec("dailyrainin")
		dewPoint           = makeGaugeVec("dewpoint")
		eventRainIn        = makeGaugeVec("eventrainin")
		feelsLike          = makeGaugeVec("feelslike")
		hourlyRainIn       = makeGaugeVec("hourlyrainin")
		maxDailyGust       = makeGaugeVec("maxdailygust")
		pm2524h            = makeGaugeVec("pm25_24h")
		monthlyRainIn      = makeGaugeVec("monthlyRainIn")
		solarRadiation     = makeGaugeVec("solarradiation")
		temp1F             = makeGaugeVec("temp1f")
		temp2F             = makeGaugeVec("temp2f")
		temp3F             = makeGaugeVec("temp3f")
		temp4F             = makeGaugeVec("temp4f")
		temp5F             = makeGaugeVec("temp5f")
		temp6F             = makeGaugeVec("temp6f")
		temp7F             = makeGaugeVec("temp7f")
		temp8F             = makeGaugeVec("temp8f")
		temp9F             = makeGaugeVec("temp9f")
		temp10F            = makeGaugeVec("temp10f")
		totalRainIn        = makeGaugeVec("totalrainin")
		uv                 = makeGaugeVec("uv")
		weeklyRainIn       = makeGaugeVec("weeklyrainin")
		windGustMPH        = makeGaugeVec("windgustmph")
		windSpeedMPH       = makeGaugeVec("windspeedmph")
		windSpeedMPHAvg2m  = makeGaugeVec("windspdmph_avg2m")
		windSpeedMPHAvg10m = makeGaugeVec("windspdmph_avg10m")
		yearlyRainIn       = makeGaugeVec("yearlyrainin")

		// integer values
		humidity      = makeGaugeVec("humidity")
		humidity1     = makeGaugeVec("humidity1")
		humidity2     = makeGaugeVec("humidity2")
		humidity3     = makeGaugeVec("humidity3")
		humidity4     = makeGaugeVec("humidity4")
		humidity5     = makeGaugeVec("humidity5")
		humidity6     = makeGaugeVec("humidity6")
		humidity7     = makeGaugeVec("humidity7")
		humidity8     = makeGaugeVec("humidity8")
		humidity9     = makeGaugeVec("humidity9")
		humidity10    = makeGaugeVec("humidity10")
		humidityIn    = makeGaugeVec("humidityin")
		pm25          = makeGaugeVec("pm25")
		relay1        = makeGaugeVec("relay1")
		relay2        = makeGaugeVec("relay2")
		relay3        = makeGaugeVec("relay3")
		relay4        = makeGaugeVec("relay4")
		relay5        = makeGaugeVec("relay5")
		relay6        = makeGaugeVec("relay6")
		relay7        = makeGaugeVec("relay7")
		relay8        = makeGaugeVec("relay8")
		relay9        = makeGaugeVec("relay9")
		relay10       = makeGaugeVec("relay10")
		windDir       = makeGaugeVec("winddir")
		windGustDir   = makeGaugeVec("windgustdir")
		windDirAvg2m  = makeGaugeVec("winddir_avg2m")
		windDirAvg10m = makeGaugeVec("winddir_avg10m")

		// batteries
		battOut = makeGaugeVec("battout")
		batt1   = makeGaugeVec("batt1")
		batt2   = makeGaugeVec("batt2")
		batt3   = makeGaugeVec("batt3")
		batt4   = makeGaugeVec("batt4")
		batt5   = makeGaugeVec("batt5")
		batt6   = makeGaugeVec("batt6")
		batt7   = makeGaugeVec("batt7")
		batt8   = makeGaugeVec("batt8")
		batt9   = makeGaugeVec("batt9")
		batt10  = makeGaugeVec("batt10")
	)

	go func() {
		for {
			dr := getAmbientDevices(key)

			for _, device := range dr.DeviceRecord {
				recordDeviceMetrics(device)

				mac := device.Macaddress

				ld := device.LastData
				// floats
				setGauge(tempInF, mac, ld.Tempinf)
				setGauge(baromAbsIn, mac, ld.Baromabsin)
				setGauge(baromRelIn, mac, ld.Baromrelin)
				setGauge(tempF, mac, ld.Tempf)
				setGauge(co2, mac, ld.Co2)
				setGauge(dailyRainIn, mac, ld.Dailyrainin)
				setGauge(dewPoint, mac, ld.Dewpoint)
				setGauge(eventRainIn, mac, ld.Eventrainin)
				setGauge(feelsLike, mac, ld.Feelslike)
				setGauge(hourlyRainIn, mac, ld.Hourlyrainin)
				setGauge(maxDailyGust, mac, ld.Maxdailygust)
				setGauge(pm2524h, mac, ld.Pm25_24h)
				setGauge(monthlyRainIn, mac, ld.Monthlyrainin)
				setGauge(solarRadiation, mac, ld.Solarradiation)
				setGauge(temp1F, mac, ld.Temp1f)
				setGauge(temp2F, mac, ld.Temp2f)
				setGauge(temp3F, mac, ld.Temp3f)
				setGauge(temp4F, mac, ld.Temp4f)
				setGauge(temp5F, mac, ld.Temp5f)
				setGauge(temp6F, mac, ld.Temp6f)
				setGauge(temp7F, mac, ld.Temp7f)
				setGauge(temp8F, mac, ld.Temp8f)
				setGauge(temp9F, mac, ld.Temp9f)
				setGauge(temp10F, mac, ld.Temp10f)
				setGauge(totalRainIn, mac, ld.Totalrainin)
				setGauge(uv, mac, ld.Uv)
				setGauge(weeklyRainIn, mac, ld.Weeklyrainin)
				setGauge(windGustMPH, mac, ld.Windgustmph)
				setGauge(windSpeedMPH, mac, ld.Windspeedmph)
				setGauge(windSpeedMPHAvg2m, mac, ld.Windspdmph_avg2m)
				setGauge(windSpeedMPHAvg10m, mac, ld.Windspdmph_avg10m)
				setGauge(yearlyRainIn, mac, ld.Yearlyrainin)

				// ints
				setGauge(humidity, mac, float64(ld.Humidity))
				setGauge(humidity1, mac, float64(ld.Humidity1))
				setGauge(humidity2, mac, float64(ld.Humidity2))
				setGauge(humidity3, mac, float64(ld.Humidity3))
				setGauge(humidity4, mac, float64(ld.Humidity4))
				setGauge(humidity5, mac, float64(ld.Humidity5))
				setGauge(humidity6, mac, float64(ld.Humidity6))
				setGauge(humidity7, mac, float64(ld.Humidity7))
				setGauge(humidity8, mac, float64(ld.Humidity8))
				setGauge(humidity9, mac, float64(ld.Humidity9))
				setGauge(humidity10, mac, float64(ld.Humidity10))
				setGauge(humidityIn, mac, float64(ld.Humidityin))
				setGauge(pm25, mac, float64(ld.Pm25))
				setGauge(relay1, mac, float64(ld.Relay1))
				setGauge(relay2, mac, float64(ld.Relay2))
				setGauge(relay3, mac, float64(ld.Relay3))
				setGauge(relay4, mac, float64(ld.Relay4))
				setGauge(relay5, mac, float64(ld.Relay5))
				setGauge(relay6, mac, float64(ld.Relay6))
				setGauge(relay7, mac, float64(ld.Relay7))
				setGauge(relay8, mac, float64(ld.Relay8))
				setGauge(relay9, mac, float64(ld.Relay9))
				setGauge(relay10, mac, float64(ld.Relay10))
				setGauge(windDir, mac, float64(ld.Winddir))
				setGauge(windGustDir, mac, float64(ld.Windgustdir))
				setGauge(windDirAvg2m, mac, float64(ld.Winddir_avg2m))
				setGauge(windDirAvg10m, mac, float64(ld.Winddir_avg10m))

				// batteries
				setBattGauge(battOut, mac, ld.Battout)
				setBattGauge(batt1, mac, ld.Batt1)
				setBattGauge(batt2, mac, ld.Batt2)
				setBattGauge(batt3, mac, ld.Batt3)
				setBattGauge(batt4, mac, ld.Batt4)
				setBattGauge(batt5, mac, ld.Batt5)
				setBattGauge(batt6, mac, ld.Batt6)
				setBattGauge(batt7, mac, ld.Batt7)
				setBattGauge(batt8, mac, ld.Batt8)
				setBattGauge(batt9, mac, ld.Batt9)
				setBattGauge(batt10, mac, ld.Batt10)
			}

			time.Sleep(60 * time.Second)
		}
	}()
}

func populateState() {
	state.GaugesByName = make(gaugeDict)
	state.LabelsByName = make(map[string](stringSet))
	state.Gauges = make(map[string](gaugeDict))
	state.Labels = make(map[string](map[string]stringDict))
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
