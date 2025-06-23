package main

import "time"

// ConnectionState represents the robot's connection state
type ConnectionState string

const (
	Online           ConnectionState = "ONLINE"
	ConnectionBroken ConnectionState = "CONNECTIONBROKEN"
	Offline          ConnectionState = "OFFLINE"
)

// RobotConnectionMessage represents the MQTT message structure for robot connection status
type RobotConnectionMessage struct {
	HeaderID        int             `json:"headerId"`
	Timestamp       string          `json:"timestamp"`
	Version         string          `json:"version"`
	Manufacturer    string          `json:"manufacturer"`
	SerialNumber    string          `json:"serialNumber"`
	ConnectionState ConnectionState `json:"connectionState"`
}

// RobotStatus holds the current status of a robot
type RobotStatus struct {
	SerialNumber    string          `json:"serialNumber"`
	Manufacturer    string          `json:"manufacturer"`
	ConnectionState ConnectionState `json:"connectionState"`
	LastUpdate      time.Time       `json:"lastUpdate"`
	LastHeaderID    int             `json:"lastHeaderId"`
	HasFactsheet    bool            `json:"hasFactsheet"`    // factsheet 수신 여부
	FactsheetUpdate time.Time       `json:"factsheetUpdate"` // factsheet 마지막 업데이트 시간
}

// PLCActionMessage represents the message from PLC bridge/actions topic
type PLCActionMessage struct {
	Action       string `json:"action"`
	SerialNumber string `json:"serialNumber,omitempty"` // Optional, if not provided, send to all online robots
}

// RobotActionMessage represents the message to robot meili/v2/Roboligent/{serial_number}/instantActions
type RobotActionMessage struct {
	HeaderID     int    `json:"headerId"`
	Timestamp    string `json:"timestamp"`
	Version      string `json:"version"`
	Manufacturer string `json:"manufacturer"`
	SerialNumber string `json:"serialNumber"`

	// For simple actions (like init and factsheetRequest)
	Actions []Action `json:"actions,omitempty"`

	// For order-based actions (like startEvisceration)
	OrderID       string `json:"orderId,omitempty"`
	OrderUpdateID int    `json:"orderUpdateId,omitempty"`
	Nodes         []Node `json:"nodes,omitempty"`
	Edges         []Edge `json:"edges,omitempty"`
}

// FactsheetRequestMessage is no longer needed - use RobotActionMessage instead

// FactsheetResponseMessage represents the factsheet response from robot
type FactsheetResponseMessage struct {
	AGVGeometry        map[string]interface{} `json:"agvGeometry"`
	HeaderID           int                    `json:"headerId"`
	Manufacturer       string                 `json:"manufacturer"`
	PhysicalParameters PhysicalParameters     `json:"physicalParameters"`
	ProtocolFeatures   ProtocolFeatures       `json:"protocolFeatures"`
	ProtocolLimits     ProtocolLimits         `json:"protocolLimits"`
	SerialNumber       string                 `json:"serialNumber"`
	Timestamp          string                 `json:"timestamp"`
	TypeSpecification  TypeSpecification      `json:"typeSpecification"`
	Version            string                 `json:"version"`
}

// PhysicalParameters represents robot physical parameters
type PhysicalParameters struct {
	AccelerationMax float64 `json:"AccelerationMax"`
	DecelerationMax float64 `json:"DecelerationMax"`
	HeightMax       float64 `json:"HeightMax"`
	HeightMin       float64 `json:"HeightMin"`
	Length          float64 `json:"Length"`
	SpeedMax        float64 `json:"SpeedMax"`
	SpeedMin        float64 `json:"SpeedMin"`
	Width           float64 `json:"Width"`
}

// ProtocolFeatures represents robot protocol features
type ProtocolFeatures struct {
	AGVActions         []AGVAction              `json:"AgvActions"`
	OptionalParameters []map[string]interface{} `json:"OptionalParameters"`
}

// AGVAction represents an available robot action
type AGVAction struct {
	ActionDescription string                `json:"ActionDescription"`
	ActionParameters  []ActionParameterSpec `json:"ActionParameters"`
	ActionScopes      []string              `json:"ActionScopes"`
	ActionType        string                `json:"ActionType"`
	ResultDescription string                `json:"ResultDescription"`
}

// ActionParameterSpec represents action parameter specification
type ActionParameterSpec struct {
	Description   string `json:"Description"`
	IsOptional    bool   `json:"IsOptional"`
	Key           string `json:"Key"`
	ValueDataType string `json:"ValueDataType"`
}

// ProtocolLimits represents protocol limitations
type ProtocolLimits struct {
	VDA5050ProtocolLimits []string `json:"VDA5050ProtocolLimits"`
}

// TypeSpecification represents robot type specification
type TypeSpecification struct {
	AGVClass          string   `json:"AgvClass"`
	AGVKinematics     string   `json:"AgvKinematics"`
	LocalizationTypes []string `json:"LocalizationTypes"`
	MaxLoadMass       int      `json:"MaxLoadMass"`
	NavigationTypes   []string `json:"NavigationTypes"`
	SeriesDescription string   `json:"SeriesDescription"`
	SeriesName        string   `json:"SeriesName"`
}

// Action represents a simple robot action (for init)
type Action struct {
	ActionType       string            `json:"actionType"`
	ActionID         string            `json:"actionId"`
	BlockingType     string            `json:"blockingType"`
	ActionParameters []ActionParameter `json:"actionParameters"`
}

// Node represents a robot navigation node (for orders)
type Node struct {
	NodeID       string       `json:"nodeId"`
	Description  string       `json:"description"`
	SequenceID   int          `json:"sequenceId"`
	Released     bool         `json:"released"`
	NodePosition NodePosition `json:"nodePosition"`
	Actions      []NodeAction `json:"actions"`
}

// NodePosition represents robot position and orientation
type NodePosition struct {
	X                     float64 `json:"x"`
	Y                     float64 `json:"y"`
	Theta                 float64 `json:"theta"`
	AllowedDeviationXY    float64 `json:"allowedDeviationXY"`
	AllowedDeviationTheta float64 `json:"allowedDeviationTheta"`
	MapID                 string  `json:"mapId"`
}

// NodeAction represents an action within a node
type NodeAction struct {
	ActionType        string            `json:"actionType"`
	ActionID          string            `json:"actionId"`
	ActionDescription string            `json:"actionDescription"`
	BlockingType      string            `json:"blockingType"`
	ActionParameters  []ActionParameter `json:"actionParameters"`
}

// Edge represents a connection between nodes
type Edge struct {
	EdgeID      string       `json:"edgeId"`
	SequenceID  int          `json:"sequenceId"`
	Released    bool         `json:"released"`
	StartNodeID string       `json:"startNodeId"`
	EndNodeID   string       `json:"endNodeId"`
	Actions     []NodeAction `json:"actions"`
}

// ActionParameter represents action parameters
type ActionParameter struct {
	Key   string      `json:"key"`
	Value interface{} `json:"value"`
}

// Pose represents robot position and orientation (for init action)
type Pose struct {
	LastNodeID string  `json:"lastNodeId"`
	MapID      string  `json:"mapId"`
	Theta      float64 `json:"theta"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
}
