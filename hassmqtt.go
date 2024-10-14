package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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

func publishHass(status interface{}, identifier string, swversion string) {
	v := reflect.ValueOf(status).Elem()
	typeOfStatus := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := typeOfStatus.Field(i)
		fieldName := field.Name
		mqttFieldName := fieldName

		unitOfMeasurement := ""
		deviceClass := ""

		if hassTag, ok := field.Tag.Lookup("hass"); ok {
			tagParts := strings.Split(hassTag, ",")
			if tagParts[0] == "-" {
				logger.Debugf("Ignoring sending field %s to HomeAssistant", fieldName)
				continue
			}

			if len(tagParts) > 0 {
				mqttFieldName = tagParts[0]
			}

			if len(tagParts) > 1 && tagParts[1] != "-" {
				unitOfMeasurement = tagParts[1]
			}

			if len(tagParts) > 2 && tagParts[2] != "-" {
				deviceClass = tagParts[2]
			}
		}

		if hassTag, ok := field.Tag.Lookup("hass"); ok && hassTag == "ignore" {
			logger.Debugf("Ignoring sending field %s to HomeAssistant", fieldName)
			continue
		}
		if hassFieldName, ok := field.Tag.Lookup("hass-name"); ok {
			mqttFieldName = hassFieldName
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

		if deviceClass != "" {
			hassConfig.DeviceClass = deviceClass
		}

		if unitOfMeasurement != "" {
			hassConfig.UnitOfMeasurement = unitOfMeasurement
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
