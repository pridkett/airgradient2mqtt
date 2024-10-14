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
	Hostname string
	Port     int
	Database string
	Username string
	Password string
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
	Serialno        string  `json:"serialno" mqtt:"-" hass:"-" influx:"mac"`
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
	Model           string  `json:"model" mqtt:"-" hass:"-" influx:"model"`
	AQI             int     `json:"aqi,omitempty" mqtt:"aqi" hass:"aqi" influx:"aqi"`
}

// set up a global logger...
// see: https://stackoverflow.com/a/43827612/57626
var logger *log.Logger

var config tomlConfig

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
	logger = log.New(os.Stderr).WithColor().WithDebug()

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
				// publishInflux(agstatus)
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

func publishInflux(status *airGradientStatus) {
}

func publishMQTT(status interface{}) {
	logger.Infof("Type of status: %v", reflect.TypeOf(status))

	v := reflect.ValueOf(status).Elem()
	typeOfStatus := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typeOfStatus.Field(i)
		fieldName := field.Name
		mqttFieldName := fieldName

		if mqttTag, ok := field.Tag.Lookup("mqtt"); ok {
			tagParts := strings.Split(mqttTag, ",")
			if tagParts[0] == "-" {
				logger.Debugf("Ignoring sending field %s to MQTT", fieldName)
				continue
			}
			if tagParts[0] != "" {
				mqttFieldName = tagParts[0]
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
