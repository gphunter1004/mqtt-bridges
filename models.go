package main

import "time"

// ConnectionState represents the robot's connection state
type ConnectionState string

const (
	Online           ConnectionState = "ONLINE"
	ConnectionBroken ConnectionState = "CONNECTIONBROKEN"
	Offline          ConnectionState = "OFFLINE"
)

// RobotConnectionMessage represents basic MQTT connection status messages
type RobotConnectionMessage struct {
	HeaderID        int             `json:"headerId"`
	Timestamp       string          `json:"timestamp"`
	Version         string          `json:"version"`
	Manufacturer    string          `json:"manufacturer"`
	SerialNumber    string          `json:"serialNumber"`
	ConnectionState ConnectionState `json:"connectionState"`
}

// RobotStateMessage represents detailed robot state messages from state topic
type RobotStateMessage struct {
	HeaderID     int    `json:"headerId"`
	Timestamp    string `json:"timestamp"`
	Version      string `json:"version"`
	Manufacturer string `json:"manufacturer"`
	SerialNumber string `json:"serialNumber"`

	// Order execution state
	OrderID       string  `json:"orderId,omitempty"`
	OrderUpdateID int     `json:"orderUpdateId,omitempty"`
	LastNodeID    string  `json:"lastNodeId,omitempty"`
	LastNodeSeqID int     `json:"lastNodeSequenceId,omitempty"`
	Driving       bool    `json:"driving"`
	Paused        bool    `json:"paused"`
	OperatingMode string  `json:"operatingMode"`
	DistanceSince float64 `json:"distanceSinceLastNode,omitempty"`

	// State arrays
	ActionStates []ActionState `json:"actionStates"`
	NodeStates   []NodeState   `json:"nodeStates"`
	EdgeStates   []EdgeState   `json:"edgeStates"`
	Errors       []ErrorInfo   `json:"errors"`
	Information  []InfoMessage `json:"information"`

	// Robot position and state
	AGVPosition    AGVPosition  `json:"agvPosition"`
	BatteryState   BatteryState `json:"batteryState"`
	SafetyState    SafetyState  `json:"safetyState"`
	Velocity       Velocity     `json:"velocity"`
	NewBaseRequest bool         `json:"newBaseRequest,omitempty"`
}

// ActionState represents the state of an action being executed
type ActionState struct {
	ActionID          string `json:"actionId"`
	ActionType        string `json:"actionType"`
	ActionDescription string `json:"actionDescription"`
	ActionStatus      string `json:"actionStatus"`
	ResultDescription string `json:"resultDescription"`
}

// NodeState represents the state of a navigation node
type NodeState struct {
	NodeID       string       `json:"nodeId"`
	SequenceID   int          `json:"sequenceId"`
	Released     bool         `json:"released"`
	NodePosition NodePosition `json:"nodePosition"`
}

// EdgeState represents the state of an edge between nodes
type EdgeState struct {
	EdgeID      string `json:"edgeId"`
	SequenceID  int    `json:"sequenceId"`
	Released    bool   `json:"released"`
	StartNodeID string `json:"startNodeId,omitempty"`
	EndNodeID   string `json:"endNodeId"`
}

// AGVPosition represents robot's current position
type AGVPosition struct {
	X                   float64 `json:"x"`
	Y                   float64 `json:"y"`
	Theta               float64 `json:"theta"`
	MapID               string  `json:"mapId"`
	MapDescription      string  `json:"mapDescription"`
	PositionInitialized bool    `json:"positionInitialized"`
	LocalizationScore   float64 `json:"localizationScore"`
	DeviationRange      float64 `json:"deviationRange"`
}

// BatteryState represents robot's battery status
type BatteryState struct {
	BatteryCharge  float64 `json:"batteryCharge"` // 실제 메시지에서 사용하는 필드명
	BatteryVoltage float64 `json:"batteryVoltage"`
	BatteryHealth  int     `json:"batteryHealth"`
	Charging       bool    `json:"charging"` // 실제 메시지에서 사용하는 필드명
	Reach          int     `json:"reach"`
}

// SafetyState represents robot's safety status
type SafetyState struct {
	EStop          string `json:"eStop"` // NONE, AUTORELEASE, MANUAL
	FieldViolation bool   `json:"fieldViolation"`
}

// Velocity represents robot's current velocity
type Velocity struct {
	VX    float64 `json:"vx"`
	VY    float64 `json:"vy"`
	Omega float64 `json:"omega"`
}

// ErrorInfo represents error information
type ErrorInfo struct {
	ErrorType        string `json:"errorType"`
	ErrorDescription string `json:"errorDescription"`
	ErrorLevel       string `json:"errorLevel"`
}

// InfoMessage represents information message
type InfoMessage struct {
	InfoType        string `json:"infoType"`
	InfoDescription string `json:"infoDescription"`
}

// RobotStatus holds the current status of a robot
type RobotStatus struct {
	SerialNumber    string          `json:"serialNumber"`
	Manufacturer    string          `json:"manufacturer"`
	ConnectionState ConnectionState `json:"connectionState"`
	LastUpdate      time.Time       `json:"lastUpdate"`
	LastHeaderID    int             `json:"lastHeaderId"`
	HasFactsheet    bool            `json:"hasFactsheet"`
	FactsheetUpdate time.Time       `json:"factsheetUpdate"`

	// Order execution state
	CurrentOrderID   string    `json:"currentOrderId,omitempty"`
	OrderUpdateID    int       `json:"orderUpdateId,omitempty"`
	OrderStartTime   time.Time `json:"orderStartTime,omitempty"`
	LastStateUpdate  time.Time `json:"lastStateUpdate,omitempty"`
	IsExecutingOrder bool      `json:"isExecutingOrder"`
	IsDriving        bool      `json:"isDriving"`
	IsPaused         bool      `json:"isPaused"`
	OperatingMode    string    `json:"operatingMode,omitempty"`

	// Position and sensor info
	CurrentPosition *AGVPosition `json:"currentPosition,omitempty"`
	BatteryLevel    float64      `json:"batteryLevel,omitempty"`
	IsCharging      bool         `json:"isCharging"`

	// Active actions and errors
	ActiveActions  []ActionState `json:"activeActions,omitempty"`
	LastError      *ErrorInfo    `json:"lastError,omitempty"`
	HasSafetyIssue bool          `json:"hasSafetyIssue"`
	LastNodeID     string        `json:"lastNodeId,omitempty"`

	// Detailed info tracking
	DetailedStatus  *RobotStateMessage `json:"detailedStatus,omitempty"`
	DetailedUpdate  time.Time          `json:"detailedUpdate"`
	HasDetailedInfo bool               `json:"hasDetailedInfo"`
	IsOnline        bool               `json:"isOnline"`
	HasErrors       bool               `json:"hasErrors"`

	// Connection vs State tracking
	HasConnectionInfo bool      `json:"hasConnectionInfo"`
	HasStateInfo      bool      `json:"hasStateInfo"`
	ConnectionUpdate  time.Time `json:"connectionUpdate"`
	StateUpdate       time.Time `json:"stateUpdate"`
}

// PLCActionMessage represents the message from PLC bridge/actions topic
type PLCActionMessage struct {
	Action       string `json:"action"`
	SerialNumber string `json:"serialNumber"` // Required in new format
}

// RobotActionMessage represents the message to robot
type RobotActionMessage struct {
	HeaderID     int    `json:"headerId"`
	Timestamp    string `json:"timestamp"`
	Version      string `json:"version"`
	Manufacturer string `json:"manufacturer"`
	SerialNumber string `json:"serialNumber"`

	// For simple actions (like init and factsheetRequest)
	Actions []Action `json:"actions,omitempty"`

	// For order-based actions (like inference and trajectory)
	OrderID       string `json:"orderId,omitempty"`
	OrderUpdateID int    `json:"orderUpdateId,omitempty"`
	Nodes         []Node `json:"nodes,omitempty"`
	Edges         []Edge `json:"edges,omitempty"`
}

// Action represents a robot action (used in both simple actions and node actions)
type Action struct {
	ActionType        string            `json:"actionType"`
	ActionID          string            `json:"actionId"`
	ActionDescription string            `json:"actionDescription,omitempty"` // Optional description
	BlockingType      string            `json:"blockingType"`
	ActionParameters  []ActionParameter `json:"actionParameters"`
}

// Node represents a robot navigation node
type Node struct {
	NodeID       string       `json:"nodeId"`
	Description  string       `json:"description"`
	SequenceID   int          `json:"sequenceId"`
	Released     bool         `json:"released"`
	NodePosition NodePosition `json:"nodePosition"`
	Actions      []Action     `json:"actions"` // Changed from []NodeAction to []Action
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

// Edge represents a connection between nodes
type Edge struct {
	EdgeID      string   `json:"edgeId"`
	SequenceID  int      `json:"sequenceId"`
	Released    bool     `json:"released"`
	StartNodeID string   `json:"startNodeId"`
	EndNodeID   string   `json:"endNodeId"`
	Actions     []Action `json:"actions"` // Changed from []NodeAction to []Action
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

// ActiveOrder represents an active robot order for monitoring
type ActiveOrder struct {
	OrderID       string    `json:"orderId"`
	IsDriving     bool      `json:"isDriving"`
	IsPaused      bool      `json:"isPaused"`
	ActiveActions int       `json:"activeActions"`
	StartTime     time.Time `json:"startTime"`
}

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
