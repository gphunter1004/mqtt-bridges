package main

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

// ActionHandler handles action conversion from PLC to Robot format
type ActionHandler struct {
	headerIDCounter int
}

// NewActionHandler creates a new action handler
func NewActionHandler() *ActionHandler {
	return &ActionHandler{
		headerIDCounter: 1,
	}
}

// generateActionID creates a unique action ID
func (ah *ActionHandler) generateActionID() string {
	// Generate random bytes
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)

	// Create hex string without dashes
	actionID := fmt.Sprintf("%x", randomBytes)

	return actionID
}

// generateOrderID creates a unique order ID
func (ah *ActionHandler) generateOrderID() string {
	// Generate random bytes for order ID
	randomBytes := make([]byte, 16)
	rand.Read(randomBytes)

	// Create hex string without dashes
	orderID := fmt.Sprintf("%x", randomBytes)

	return orderID
}

// getNextHeaderID returns the next header ID
func (ah *ActionHandler) getNextHeaderID() int {
	ah.headerIDCounter++
	return ah.headerIDCounter
}

// createBaseRobotMessage creates a base robot message with common fields
func (ah *ActionHandler) createBaseRobotMessage(serialNumber string, manufacturer string) *RobotActionMessage {
	return &RobotActionMessage{
		HeaderID:     ah.getNextHeaderID(),
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Version:      "2.0.0",
		Manufacturer: manufacturer,
		SerialNumber: serialNumber,
	}
}

// createBaseNodePosition creates a base node position with default values
func (ah *ActionHandler) createBaseNodePosition() NodePosition {
	return NodePosition{
		X:                     0.0,
		Y:                     0.0,
		Theta:                 0.0,
		AllowedDeviationXY:    0.5,
		AllowedDeviationTheta: 0.17453292, // 10 degrees in radians
		MapID:                 "floor 0",
	}
}

// createInferenceNodePosition creates a specific position for inference actions
func (ah *ActionHandler) createInferenceNodePosition() NodePosition {
	return NodePosition{
		X:                     -4.16,
		Y:                     -0.39,
		Theta:                 3.1415927, // 180 degrees in radians
		AllowedDeviationXY:    0.5,
		AllowedDeviationTheta: 0.17453292, // 10 degrees in radians
		MapID:                 "floor 0",
	}
}

// createTrajectoryNodePosition creates a specific position for trajectory actions
// Based on actual system data, trajectory uses the same position as inference
func (ah *ActionHandler) createTrajectoryNodePosition() NodePosition {
	return NodePosition{
		X:                     -4.16,
		Y:                     -0.39,
		Theta:                 3.1415927, // 180 degrees in radians
		AllowedDeviationXY:    0.5,
		AllowedDeviationTheta: 0.17453292, // 10 degrees in radians
		MapID:                 "floor 0",
	}
}

// ConvertPLCActionToRobotAction converts PLC action message to robot action message
func (ah *ActionHandler) ConvertPLCActionToRobotAction(plcAction *PLCActionMessage, serialNumber string) (*RobotActionMessage, error) {
	switch plcAction.Action {
	case "init":
		return ah.createInitPositionAction(serialNumber), nil
	case "factsheetRequest":
		return ah.createFactsheetRequestAction(serialNumber, "Roboligent"), nil
	default:
		// Check if it's an inference action
		if strings.HasPrefix(plcAction.Action, "inference:") {
			inferenceName := strings.TrimPrefix(plcAction.Action, "inference:")
			if inferenceName == "" {
				return nil, fmt.Errorf("inference name is required for inference action")
			}
			return ah.createInferenceAction(serialNumber, inferenceName), nil
		}
		// Check if it's a trajectory action
		if strings.HasPrefix(plcAction.Action, "trajectory:") {
			trajectoryName := strings.TrimPrefix(plcAction.Action, "trajectory:")
			if trajectoryName == "" {
				return nil, fmt.Errorf("trajectory name is required for trajectory action")
			}
			return ah.createTrajectoryAction(serialNumber, trajectoryName), nil
		}
		return nil, fmt.Errorf("unsupported action type: %s", plcAction.Action)
	}
}

// createInitPositionAction creates an init position action for the robot
func (ah *ActionHandler) createInitPositionAction(serialNumber string) *RobotActionMessage {
	// Create pose with default values (origin position)
	pose := Pose{
		LastNodeID: "",
		MapID:      "",
		Theta:      0.0,
		X:          0.0,
		Y:          0.0,
	}

	// Create action parameter
	actionParam := ActionParameter{
		Key:   "pose",
		Value: pose,
	}

	// Create action
	action := Action{
		ActionType:       "initPosition",
		ActionID:         ah.generateActionID(),
		BlockingType:     "NONE",
		ActionParameters: []ActionParameter{actionParam},
	}

	// Create robot action message (simple format for init)
	robotAction := ah.createBaseRobotMessage(serialNumber, "Roboligent")
	robotAction.Actions = []Action{action}

	return robotAction
}

// createFactsheetRequestAction creates a factsheet request action for the robot
func (ah *ActionHandler) createFactsheetRequestAction(serialNumber string, manufacturer string) *RobotActionMessage {
	// Create factsheet request action (no parameters needed)
	action := Action{
		ActionType:       "factsheetRequest",
		ActionID:         ah.generateActionID(),
		BlockingType:     "NONE",
		ActionParameters: []ActionParameter{}, // Empty parameters
	}

	// Create robot action message (simple format)
	robotAction := ah.createBaseRobotMessage(serialNumber, manufacturer)
	robotAction.Actions = []Action{action}

	return robotAction
}

// createInferenceAction creates an inference action for the robot
func (ah *ActionHandler) createInferenceAction(serialNumber string, inferenceName string) *RobotActionMessage {
	// Create intermediate node (starting point)
	intermediateNode := Node{
		NodeID:       "intermediate_node_0_0",
		Description:  fmt.Sprintf("intermediate point 0 of task inference-%s subtask index 0", inferenceName),
		SequenceID:   0,
		Released:     true,
		NodePosition: ah.createBaseNodePosition(),
		Actions:      []NodeAction{}, // Empty actions for intermediate node
	}

	// Create inference action
	inferenceAction := NodeAction{
		ActionType:        "Roboligent Robin - Inference",
		ActionID:          ah.generateActionID(),
		ActionDescription: "This is an action will trigger the behavior tree for executing inference.",
		BlockingType:      "NONE",
		ActionParameters: []ActionParameter{
			{
				Key:   "inference_name",
				Value: inferenceName,
			},
		},
	}

	// Create inference node
	inferenceNode := Node{
		NodeID:       ah.generateActionID(),
		Description:  fmt.Sprintf("we are in 2 Subtask of inference-%s at index 0", inferenceName),
		SequenceID:   2,
		Released:     true,
		NodePosition: ah.createInferenceNodePosition(),
		Actions:      []NodeAction{inferenceAction},
	}

	// Create edge connecting the nodes
	edge := Edge{
		EdgeID:      "intermediate_edge_0_0",
		SequenceID:  1,
		Released:    true,
		StartNodeID: "intermediate_node_0_0",
		EndNodeID:   inferenceNode.NodeID,
		Actions:     []NodeAction{}, // Empty actions for edge
	}

	// Create robot action message (order format)
	robotAction := ah.createBaseRobotMessage(serialNumber, "Roboligent")
	robotAction.OrderID = ah.generateOrderID()
	robotAction.OrderUpdateID = 0
	robotAction.Nodes = []Node{intermediateNode, inferenceNode}
	robotAction.Edges = []Edge{edge}

	return robotAction
}

// createTrajectoryAction creates a trajectory action for the robot
// Updated to match the actual system behavior with intermediate nodes and specific position
func (ah *ActionHandler) createTrajectoryAction(serialNumber string, trajectoryName string) *RobotActionMessage {
	// Create intermediate node (starting point)
	intermediateNode := Node{
		NodeID:       "intermediate_node_0_0",
		Description:  fmt.Sprintf("intermediate point 0 of task trajectory-%s subtask index 0", trajectoryName),
		SequenceID:   0,
		Released:     true,
		NodePosition: ah.createBaseNodePosition(),
		Actions:      []NodeAction{}, // Empty actions for intermediate node
	}

	// Create trajectory action
	trajectoryAction := NodeAction{
		ActionType:        "Roboligent Robin - Follow Trajectory",
		ActionID:          ah.generateActionID(),
		ActionDescription: "This action will trigger the behavior tree for following a recorded trajectory.",
		BlockingType:      "NONE",
		ActionParameters: []ActionParameter{
			{
				Key:   "arm",
				Value: "right", // Default arm setting, matches actual system data
			},
			{
				Key:   "trajectory_name",
				Value: trajectoryName,
			},
		},
	}

	// Create trajectory execution node
	// Updated to use specific position like inference (matches actual system behavior)
	trajectoryNode := Node{
		NodeID:       ah.generateActionID(),
		Description:  fmt.Sprintf("we are in 2 Subtask of trajectory-%s at index 0", trajectoryName),
		SequenceID:   2, // Updated to match actual system (was 0)
		Released:     true,
		NodePosition: ah.createTrajectoryNodePosition(), // Updated to use specific position
		Actions:      []NodeAction{trajectoryAction},
	}

	// Create edge connecting the nodes
	// Updated to include edge (was missing in original implementation)
	edge := Edge{
		EdgeID:      "intermediate_edge_0_0",
		SequenceID:  1,
		Released:    true,
		StartNodeID: "intermediate_node_0_0",
		EndNodeID:   trajectoryNode.NodeID,
		Actions:     []NodeAction{}, // Empty actions for edge
	}

	// Create robot action message (order format)
	robotAction := ah.createBaseRobotMessage(serialNumber, "Roboligent")
	robotAction.OrderID = ah.generateOrderID()
	robotAction.OrderUpdateID = 0
	robotAction.Nodes = []Node{intermediateNode, trajectoryNode} // Updated to include both nodes
	robotAction.Edges = []Edge{edge}                             // Updated to include edge

	return robotAction
}

// createConfigurableTrajectoryAction creates a trajectory action with configurable parameters
// This method allows for more flexibility in trajectory execution
func (ah *ActionHandler) createConfigurableTrajectoryAction(serialNumber string, trajectoryName string, targetPosition NodePosition, armSetting string) *RobotActionMessage {
	// Create intermediate node
	intermediateNode := Node{
		NodeID:       "intermediate_node_0_0",
		Description:  fmt.Sprintf("intermediate point 0 of task trajectory-%s subtask index 0", trajectoryName),
		SequenceID:   0,
		Released:     true,
		NodePosition: ah.createBaseNodePosition(),
		Actions:      []NodeAction{},
	}

	// Create trajectory action with configurable arm
	trajectoryAction := NodeAction{
		ActionType:        "Roboligent Robin - Follow Trajectory",
		ActionID:          ah.generateActionID(),
		ActionDescription: "This action will trigger the behavior tree for following a recorded trajectory.",
		BlockingType:      "NONE",
		ActionParameters: []ActionParameter{
			{
				Key:   "arm",
				Value: armSetting, // Configurable: "right", "left", "both"
			},
			{
				Key:   "trajectory_name",
				Value: trajectoryName,
			},
		},
	}

	// Create trajectory execution node with configurable position
	trajectoryNode := Node{
		NodeID:       ah.generateActionID(),
		Description:  fmt.Sprintf("we are in 2 Subtask of trajectory-%s at index 0", trajectoryName),
		SequenceID:   2,
		Released:     true,
		NodePosition: targetPosition, // Configurable position
		Actions:      []NodeAction{trajectoryAction},
	}

	// Create edge
	edge := Edge{
		EdgeID:      "intermediate_edge_0_0",
		SequenceID:  1,
		Released:    true,
		StartNodeID: "intermediate_node_0_0",
		EndNodeID:   trajectoryNode.NodeID,
		Actions:     []NodeAction{},
	}

	// Create robot action message
	robotAction := ah.createBaseRobotMessage(serialNumber, "Roboligent")
	robotAction.OrderID = ah.generateOrderID()
	robotAction.OrderUpdateID = 0
	robotAction.Nodes = []Node{intermediateNode, trajectoryNode}
	robotAction.Edges = []Edge{edge}

	return robotAction
}

// ValidatePLCAction validates the PLC action message
func ValidatePLCAction(plcAction *PLCActionMessage) error {
	if plcAction.Action == "" {
		return fmt.Errorf("action is required")
	}

	// Validate known actions
	switch plcAction.Action {
	case "init", "factsheetRequest":
		return nil
	default:
		// Check parametric actions
		if strings.HasPrefix(plcAction.Action, "inference:") {
			inferenceName := strings.TrimPrefix(plcAction.Action, "inference:")
			if inferenceName == "" {
				return fmt.Errorf("inference name is required for inference action")
			}
			return nil
		}
		if strings.HasPrefix(plcAction.Action, "trajectory:") {
			trajectoryName := strings.TrimPrefix(plcAction.Action, "trajectory:")
			if trajectoryName == "" {
				return fmt.Errorf("trajectory name is required for trajectory action")
			}
			return nil
		}
		return fmt.Errorf("unknown action type: %s", plcAction.Action)
	}
}

// ParsePLCActionMessage parses PLC action message
// Handles format: {serial}:action (e.g., "DEX0002:init" or "DEX0002:inference:inference1")
func ParsePLCActionMessage(payload []byte) (*PLCActionMessage, error) {
	payloadStr := strings.TrimSpace(string(payload))

	// Check if payload contains serial:action format
	if strings.Contains(payloadStr, ":") {
		parts := strings.SplitN(payloadStr, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format: expected 'serial:action', got '%s'", payloadStr)
		}

		serial := strings.TrimSpace(parts[0])
		action := strings.TrimSpace(parts[1])

		if serial == "" || action == "" {
			return nil, fmt.Errorf("empty serial or action in '%s'", payloadStr)
		}

		return &PLCActionMessage{
			Action:       action,
			SerialNumber: serial,
		}, nil
	}

	// Handle legacy format (backwards compatibility)
	switch payloadStr {
	case "init":
		return &PLCActionMessage{
			Action: "init",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported PLC action format: '%s'. Expected format: 'serial:action', 'serial:inference:name', 'serial:trajectory:name' or 'init'", payloadStr)
	}
}
