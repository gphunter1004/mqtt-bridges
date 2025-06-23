package main

import (
	"fmt"
	"strings"
)

// parseRobotConnectionTopic extracts serial number from topic
func parseRobotConnectionTopic(topic string) (string, error) {
	// Topic format: meili/v2/Roboligent/{serial_number}/connection
	parts := strings.Split(topic, "/")
	if len(parts) != 5 || parts[0] != "meili" || parts[1] != "v2" || parts[2] != "Roboligent" || parts[4] != "connection" {
		return "", fmt.Errorf("invalid topic format: %s", topic)
	}
	return parts[3], nil
}

// buildRobotConnectionTopic builds a robot connection topic for a given serial number
func buildRobotConnectionTopic(serialNumber string) string {
	return fmt.Sprintf("meili/v2/Roboligent/%s/connection", serialNumber)
}

// buildRobotActionTopic builds a robot instant action topic for a given serial number
func buildRobotActionTopic(serialNumber string) string {
	return fmt.Sprintf("meili/v2/Roboligent/%s/instantActions", serialNumber)
}

// buildRobotFactsheetTopic builds a robot factsheet topic for a given serial number and manufacturer
func buildRobotFactsheetTopic(serialNumber string, manufacturer string) string {
	return fmt.Sprintf("meili/v2/%s/%s/factsheet", manufacturer, serialNumber)
}

// validateRobotMessage validates the robot connection message
func validateRobotMessage(msg *RobotConnectionMessage) error {
	if msg.SerialNumber == "" {
		return fmt.Errorf("serial number is required")
	}
	if msg.Manufacturer == "" {
		return fmt.Errorf("manufacturer is required")
	}
	if msg.Version == "" {
		return fmt.Errorf("version is required")
	}
	if msg.ConnectionState != Online && msg.ConnectionState != ConnectionBroken && msg.ConnectionState != Offline {
		return fmt.Errorf("invalid connection state: %s", msg.ConnectionState)
	}
	return nil
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

// validateRobotActionMessage validates the robot action message
func validateRobotActionMessage(msg *RobotActionMessage) error {
	if msg.SerialNumber == "" {
		return fmt.Errorf("serial number is required")
	}
	if msg.Manufacturer == "" {
		return fmt.Errorf("manufacturer is required")
	}
	if msg.Version == "" {
		return fmt.Errorf("version is required")
	}

	// Check if it's a simple action format (has Actions field)
	if len(msg.Actions) > 0 {
		// Validate each action
		for i, action := range msg.Actions {
			if action.ActionType == "" {
				return fmt.Errorf("action[%d]: actionType is required", i)
			}
			if action.ActionID == "" {
				return fmt.Errorf("action[%d]: actionId is required", i)
			}
			if action.BlockingType == "" {
				return fmt.Errorf("action[%d]: blockingType is required", i)
			}
		}
		return nil
	}

	// Check if it's an order format (has Nodes field)
	if len(msg.Nodes) > 0 {
		if msg.OrderID == "" {
			return fmt.Errorf("orderId is required for order format")
		}
		// Validate nodes
		for i, node := range msg.Nodes {
			if node.NodeID == "" {
				return fmt.Errorf("node[%d]: nodeId is required", i)
			}
		}
		return nil
	}

	// Neither Actions nor Nodes are present
	return fmt.Errorf("either actions or nodes must be provided")
}
