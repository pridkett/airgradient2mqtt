package main

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Types for Home Assistant MQTT Discovery
type hassMqttConfigDevice struct {
	Identifiers  []string `json:"identifiers"`
	Manufacturer string   `json:"manufacturer"`
	Model        string   `json:"model"`
	Name         string   `json:"name"`
	SWVersion    string   `json:"sw_version"`
}

type hassMqttConfig struct {
	AvailabilityTopic string               `json:"availability_topic"`
	ConfigTopic       string               `json:"-"`
	Device            hassMqttConfigDevice `json:"device"`
	DeviceClass       string               `json:"device_class,omitempty"`
	Name              string               `json:"name"`
	Qos               int                  `json:"qos"`
	StateTopic        string               `json:"state_topic"`
	UniqueId          string               `json:"unique_id"`
	Icon              string               `json:"icon,omitempty"`
	UnitOfMeasurement string               `json:"unit_of_measurement"`
	Platform          string               `json:"-"`
}

var HASS_TAG_LABELS = []string{"name", "unit", "class", "icon"}

func publishHass(status interface{}, identifier string, swversion string) {
	v := reflect.ValueOf(status).Elem()
	typeOfStatus := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typeOfStatus.Field(i)
		fieldName := field.Name
		mqttFieldName := fieldName

		hassTags := getFieldTags(field, "hass", HASS_TAG_LABELS)
		if _, ok := hassTags["name"]; ok {
			mqttFieldName = hassTags["name"]
			if mqttFieldName == "-" {
				logger.Debugf("Ignoring sending field %s to HomeAssistant", fieldName)
				continue
			}
		}

		// generage the topic name
		topic := fmt.Sprintf("%s/%s/%s/%s", config.Hass.DiscoveryPrefix, "sensor", config.Hass.DeviceName, mqttFieldName)

		// send the availabilty message
		token := client.Publish(topic+"/availability", 0, false, "online")
		token.Wait()

		// send the state message
		token = client.Publish(topic+"/state", 0, false, fmt.Sprintf("%v", reflect.ValueOf(status).Elem().FieldByName(fieldName).Interface()))
		token.Wait()

		// send the config message
		hassConfig := hassMqttConfig{
			AvailabilityTopic: topic + "/availability",
			ConfigTopic:       topic + "/config",
			Device: hassMqttConfigDevice{
				Identifiers:  []string{identifier},
				Manufacturer: config.Hass.Manufacturer,
				Model:        config.Hass.DeviceModel,
				Name:         config.Hass.DeviceName,
			},
			Name:       fieldName,
			Qos:        0,
			StateTopic: topic + "/state",
			UniqueId:   fmt.Sprintf("%s_%s", identifier, mqttFieldName),
		}

		if swversion != "" {
			hassConfig.Device.SWVersion = swversion
		}

		if _, ok := hassTags["class"]; ok {
			deviceClass := hassTags["class"]
			if deviceClass != "-" {
				hassConfig.DeviceClass = deviceClass
			}
		}

		if _, ok := hassTags["unit"]; ok {
			unitOfMeasurement := hassTags["unit"]
			if unitOfMeasurement != "-" {
				hassConfig.UnitOfMeasurement = unitOfMeasurement
			}
		}

		if _, ok := hassTags["icon"]; ok {
			icon := hassTags["icon"]
			if icon != "-" {
				hassConfig.Icon = icon
			}
		}

		configPayload, err := json.Marshal(hassConfig)
		if err != nil {
			logger.Errorf("Error marshalling hassConfig to JSON: %v", err)
			continue
		}
		token = client.Publish(topic+"/config", 0, false, configPayload)
		token.Wait()
	}
}
