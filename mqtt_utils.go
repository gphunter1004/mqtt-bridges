package main

import (
	"fmt"
	"strings"
)

// parseRobotConnectionTopic extracts serial number from robot connection topic
func parseRobotConnectionTopic(topic string) (string, error) {
	// Topic format: meili/v2/Roboligent/{serial_number}/connection
	parts := strings.Split(topic, "/")
	if len(parts) != 5 || parts[0] != "meili" || parts[1] != "v2" || parts[2] != "Roboligent" || parts[4] != "connection" {
		return "", fmt.Errorf("invalid connection topic format: %s", topic)
	}
	return parts[3], nil
}

// parseRobotStateTopic extracts serial number from robot state topic
func parseRobotStateTopic(topic string) (string, error) {
	// Topic format: meili/v2/Roboligent/{serial_number}/state
	parts := strings.Split(topic, "/")
	if len(parts) != 5 || parts[0] != "meili" || parts[1] != "v2" || parts[2] != "Roboligent" || parts[4] != "state" {
		return "", fmt.Errorf("invalid state topic format: %s", topic)
	}
	return parts[3], nil
}

// parseRobotFactsheetTopic extracts serial number and manufacturer from factsheet topic
func parseRobotFactsheetTopic(topic string) (string, string, error) {
	// Topic format: meili/v2/{manufacturer}/{serial_number}/factsheet
	parts := strings.Split(topic, "/")
	if len(parts) != 5 || parts[0] != "meili" || parts[1] != "v2" || parts[4] != "factsheet" {
		return "", "", fmt.Errorf("invalid factsheet topic format: %s", topic)
	}
	manufacturer := parts[2]
	serialNumber := parts[3]
	return serialNumber, manufacturer, nil
}

// buildRobotActionTopic builds a robot instant action topic for a given serial number
func buildRobotActionTopic(serialNumber string) string {
	return fmt.Sprintf("meili/v2/Roboligent/%s/instantActions", serialNumber)
}

// buildRobotOrderTopic builds a robot order topic for a given serial number
func buildRobotOrderTopic(serialNumber string) string {
	return fmt.Sprintf("meili/v2/Roboligent/%s/orders", serialNumber)
}
