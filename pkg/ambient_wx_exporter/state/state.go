package state

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/dantswain/ambient_wx_exporter/pkg/ambient_wx_exporter/config"
)

type gaugeDict map[string](*prometheus.GaugeVec)
type stringDict map[string]string
type stringSet map[string](struct{})

// State represents the application state
type State struct {
	AppKey        string
	APIKey        string
	Gauges        map[string]gaugeDict
	Labels        map[string](map[string](stringDict))
	DefaultGauges gaugeDict
	DataAgeGauge  *prometheus.GaugeVec
	RainAgeGauge  *prometheus.GaugeVec
}

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

// Init the state from config
func Init(metricPrefix string, appKey string, apiKey string, theConfig *config.Config, disableDefaultGauges bool) *State {
	state := &State{}

	state.AppKey = appKey
	state.APIKey = apiKey

	gaugesByName := make(gaugeDict)
	labelsByName := make(map[string](stringSet))
	state.Gauges = make(map[string](gaugeDict))
	state.Labels = make(map[string](map[string]stringDict))
	state.DefaultGauges = make(gaugeDict)

	for _, d := range theConfig.Devices {
		for _, g := range d.Gauges {
			_, ok := labelsByName[g.Name]
			if !ok {
				labelsByName[g.Name] = make(map[string](struct{}))
			}
			for k := range g.Labels {
				labelsByName[g.Name][k] = struct{}{}
			}
			labelsByName[g.Name]["mac_address"] = struct{}{}
		}
	}

	for n, l := range labelsByName {
		labels := make([]string, len(l))
		ix := 0
		for ll := range l {
			labels[ix] = ll
			ix++
		}

		gaugesByName[n] = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: metricPrefix + n,
		}, labels)
	}

	for _, d := range theConfig.Devices {
		state.Gauges[d.MacAddress] = make(map[string](*prometheus.GaugeVec))
		state.Labels[d.MacAddress] = make(map[string]stringDict)
		for _, g := range d.Gauges {
			state.Gauges[d.MacAddress][g.APIName] = gaugesByName[g.Name]
			labels := g.Labels
			labels["mac_address"] = d.MacAddress
			state.Labels[d.MacAddress][g.APIName] = labels
		}
	}

	if !disableDefaultGauges {
		for _, n := range apiKeys {
			state.DefaultGauges[n] = promauto.NewGaugeVec(prometheus.GaugeOpts{
				Name: metricPrefix + n,
			}, []string{"mac_address"})
		}
	}

	state.DataAgeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricPrefix + "data_age",
	}, []string{"mac_address"})

	state.RainAgeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: metricPrefix + "time_since_rain",
	}, []string{"mac_address"})

	return state
}
