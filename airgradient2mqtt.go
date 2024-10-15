package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/naoina/toml"
	"github.com/withmandala/go-log"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	influxclient "github.com/influxdata/influxdb1-client/v2"
)

// MQTT settings for overall configuration
type tomlConfigMQTT struct {
	BrokerHost     string
	BrokerPort     int
	BrokerUsername string
	BrokerPassword string
	ClientId       string
	TopicPrefix    string
	Topic          string
}

type tomlConfigHass struct {
	Discovery       bool
	DiscoveryPrefix string
	ObjectId        string
	DeviceModel     string
	DeviceName      string
	Manufacturer    string
}

type tomlConfigInflux struct {
	Hostname    string
	Port        int
	Database    string
	Username    string
	Password    string
	Measurement string
}

type tomlConfigAirGradient struct {
	Url      string
	PollRate int
}

type tomlConfig struct {
	AirGradient tomlConfigAirGradient
	Mqtt        tomlConfigMQTT
	Hass        tomlConfigHass
	Influx      tomlConfigInflux
}

// AirGradient data structure
type airGradientStatus struct {
	Wifi            int     `json:"wifi" mqtt:"-" hass:"-" influx:"wifi"`
	Serialno        string  `json:"serialno" mqtt:"-" hass:"-" influx:"-"`
	Rco2            int     `json:"rco2" mqtt:"rco2" hass:"rco2,ppm" influx:"rco2"`
	Pm01            int     `json:"pm01" mqtt:"pm01" hass:"pm01,µg/m³" influx:"pm01"`
	Pm02            int     `json:"pm02" mqtt:"pm02" hass:"pm02,µg/m³" influx:"pm02"`
	Pm10            int     `json:"pm10" mqtt:"pm10" hass:"pm10,µg/m³" influx:"pm10"`
	Pm003count      int     `json:"pm003count" mqtt:"pm003count" hass:"pm003count,particles/0.1L" influx:"pm003_count"`
	Atmp            float64 `json:"atmp" mqtt:"atmp" hass:"atmp,°C" influx:"atmp"`
	AtmpCompensated float64 `json:"atmpCompensated" mqtt:"atmpCompensated" hass:"atmpCompensated,°C" influx:"atmp_compensated"`
	Rhum            float64 `json:"rhum" mqtt:"rhum" hass:"rhum,%" influx:"rhum"`
	RhumCompensated float64 `json:"rhumCompensated" mqtt:"rhumCompensated" hass:"rhumCompensated,%" influx:"rhum_compensated"`
	Pm02Compensated int     `json:"pm02Compensated" mqtt:"pm02Compensated" hass:"pm02Compensated,µg/m³" influx:"pm02_compensated"`
	TvocIndex       int     `json:"tvocIndex" mqtt:"tvocIndex" hass:"tvocIndex,ppb" influx:"tvoc_index"`
	TvocRaw         int     `json:"tvocRaw" mqtt:"tvocRaw" hass:"tvocRaw,ppb" influx:"tvoc_raw"`
	NoxIndex        int     `json:"noxIndex" mqtt:"noxIndex" hass:"noxIndex,ppb" influx:"nox_index"`
	NoxRaw          int     `json:"noxRaw" mqtt:"noxRaw" hass:"noxRaw" influx:"nox_raw"`
	Boot            int     `json:"boot" mqtt:"-" hass:"-" influx:"boot"`
	BootCount       int     `json:"bootCount" mqtt:"-" hass:"-" influx:"boot_count"`
	LedMode         string  `json:"ledMode" mqtt:"-" hass:"-" influx:"led_mode"`
	Firmware        string  `json:"firmware" mqtt:"-" hass:"-" influx:"firmware"`
	Model           string  `json:"model" mqtt:"-" hass:"-" influx:"-"`
	AQI             int     `json:"aqi,omitempty" mqtt:"aqi" hass:"aqi" influx:"aqi"`
}

// set up a global logger...
// see: https://stackoverflow.com/a/43827612/57626
var logger *log.Logger

var config tomlConfig

var MQTT_TAG_LABELS = []string{"name"}
var INFLUX_TAG_LABELS = []string{"name"}

// var components tomlComponents
var client mqtt.Client

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	r := client.OptionsReader()
	logger.Infof("Connected to MQTT at %s", r.Servers())
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	logger.Errorf("MQTT Connection lost: %v", err)
}

func main() {
	logger = log.New(os.Stderr).WithColor()

	configFile := flag.String("config", "", "Filename with configuration")
	flag.Parse()

	if *configFile != "" {
		f, err := os.Open(*configFile)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := toml.NewDecoder(f).Decode(&config); err != nil {
			panic(err)
		}
	} else {
		logger.Fatal("Must specify configuration file with -config FILENAME")
	}

	if config.Mqtt != (tomlConfigMQTT{}) {
		mqttConnect()
	} else {
		logger.Info("No MQTT configuration found - not publishing to MQTT broker")
		if config.Hass != (tomlConfigHass{}) {
			logger.Fatal("Hass configuration found but no MQTT configuration found - please configure MQTT broker")
		}
	}

	logger.Infof("HTTP Target: %s", config.AirGradient.Url)
	var myClient = &http.Client{Timeout: 10 * time.Second}

	for {
		agstatus := new(airGradientStatus)
		// see: https://stackoverflow.com/a/31129967/57626
		getJson(config.AirGradient.Url, agstatus, myClient)
		// calculate PM2.5 AQI and add it into the struct
		agstatus.AQI = PM25toAQI(agstatus.Pm02Compensated)

		if agstatus.Serialno != "" {
			if config.Influx != (tomlConfigInflux{}) {
				tags := map[string]string{
					"mac":   agstatus.Serialno,
					"model": agstatus.Model,
				}
				publishInflux(agstatus, config.Influx.Measurement, tags)
			}

			if config.Mqtt != (tomlConfigMQTT{}) {
				if config.Mqtt.Topic == "" {
					config.Mqtt.Topic = "airgradient-" + agstatus.Serialno
				}
				publishMQTT(agstatus)
			}

			if config.Hass != (tomlConfigHass{}) {
				publishHass(agstatus, agstatus.Serialno, agstatus.Firmware)
			}
		} else {
			logger.Warnf("Got a strange response from the AirGradient API - skipping this poll")
		}
		logger.Debugf("Sleeping for %d seconds", config.AirGradient.PollRate)
		time.Sleep(time.Duration(config.AirGradient.PollRate) * time.Second)
	}
}

func mqttConnect() {
	opts := mqtt.NewClientOptions()

	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.Mqtt.BrokerHost, config.Mqtt.BrokerPort))
	if config.Mqtt.BrokerPassword != "" && config.Mqtt.BrokerUsername != "" {
		opts.SetUsername(config.Mqtt.BrokerUsername)
		opts.SetPassword(config.Mqtt.BrokerPassword)
	}
	opts.SetClientID(config.Mqtt.ClientId)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler

	client = mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	if config.Mqtt.TopicPrefix == "" {
		config.Mqtt.TopicPrefix = "airgradient"
	}
}

func getJson(url string, target interface{}, myClient *http.Client) error {
	r, err := myClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func getFieldTags(field reflect.StructField, lookupKey string, defaultLabels []string) map[string]string {
	tags := make(map[string]string)
	labellessTagsValid := true

	if tag, ok := field.Tag.Lookup(lookupKey); ok {
		tagParts := strings.Split(tag, ",")
		for i, tag := range tagParts {
			splitTag := strings.Split(tag, ":")
			if len(splitTag) == 1 {
				if labellessTagsValid {
					if i < len(defaultLabels) {
						tags[defaultLabels[i]] = splitTag[0]
					} else {
						logger.Errorf("Invalid tag - too many labelless tags: %s", tag)
					}
				} else {
					logger.Errorf("Invalid tag - labelless tags not allowed after labeled tag: %s", tag)
				}
			} else if len(splitTag) == 2 {
				labellessTagsValid = false
				tags[splitTag[0]] = splitTag[1]
			} else {
				logger.Errorf("Invalid tag - too man parts: %s", tag)
			}
		}
	}
	return tags
}

func publishInflux(status interface{}, measurement string, tags map[string]string) {
	logger.Infof("Type of status: %v", reflect.TypeOf(status))

	v := reflect.ValueOf(status).Elem()
	typeOfStatus := v.Type()

	httpConfig := influxclient.HTTPConfig{
		Addr: fmt.Sprintf("http://%s:%d", config.Influx.Hostname, config.Influx.Port),
	}
	if config.Influx.Username != "" && config.Influx.Password != "" {
		httpConfig.Username = config.Influx.Username
		httpConfig.Password = config.Influx.Password
	}

	c, err := influxclient.NewHTTPClient(httpConfig)
	if err != nil {
		logger.Errorf("Error creating InfluxDB Client: ", err.Error())
	}
	defer c.Close()

	bp, err := influxclient.NewBatchPoints(influxclient.BatchPointsConfig{
		Database:  config.Influx.Database,
		Precision: "s",
	})
	if err != nil {
		logger.Errorf("error creating batchpoints: %s", err)
	}

	values := map[string]interface{}{}

	for i := 0; i < v.NumField(); i++ {
		field := typeOfStatus.Field(i)
		fieldName := field.Name
		influxFieldName := fieldName

		influxTags := getFieldTags(field, "influx", INFLUX_TAG_LABELS)

		if _, ok := influxTags["name"]; ok {
			influxFieldName = influxTags["name"]
			if influxFieldName == "-" {
				continue
			}
		}

		values[influxFieldName] = v.Field(i).Interface()
	}

	point, err := influxclient.NewPoint(measurement, tags, values, time.Now())
	if err != nil {
		logger.Errorf("error creating new point: %s", err)
	}
	bp.AddPoint(point)
	err = c.Write(bp)

	if err != nil {
		logger.Fatal(err)
	}
	logger.Infof("Record published to InfluxDB")

}

func publishMQTT(status interface{}) {
	logger.Infof("Type of status: %v", reflect.TypeOf(status))

	v := reflect.ValueOf(status).Elem()
	typeOfStatus := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typeOfStatus.Field(i)
		fieldName := field.Name
		mqttFieldName := fieldName

		mqttTags := getFieldTags(field, "mqtt", MQTT_TAG_LABELS)
		if _, ok := mqttTags["name"]; ok {
			mqttFieldName = mqttTags["name"]
			if mqttFieldName == "-" {
				logger.Debugf("Ignoring sending field %s to MQTT", fieldName)
				continue
			}
		}

		fieldValue := v.Field(i).Interface()
		topic := fmt.Sprintf("%s/%s/%s", config.Mqtt.TopicPrefix, config.Mqtt.Topic, mqttFieldName)
		logger.Debugf("field[%s] = [%v]", fieldName, fieldValue)
		logger.Debugf("topic = %s", topic)
		token := client.Publish(topic, 0, false, fmt.Sprintf("%v", fieldValue))
		token.Wait()
	}
}
