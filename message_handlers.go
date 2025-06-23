package main

import (
	"encoding/json"
	"fmt"
	"log"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MessageProcessor handles all MQTT message processing
type MessageProcessor struct {
	mqttClient    *MQTTClient
	robotManager  *RobotManager
	actionHandler *ActionHandler
	config        *Config
}

// NewMessageProcessor creates a new message processor
func NewMessageProcessor(mqttClient *MQTTClient, robotManager *RobotManager, actionHandler *ActionHandler, config *Config) *MessageProcessor {
	return &MessageProcessor{
		mqttClient:    mqttClient,
		robotManager:  robotManager,
		actionHandler: actionHandler,
		config:        config,
	}
}

// GetMessageHandlers returns handlers for all message types
func (mp *MessageProcessor) GetMessageHandlers() *MessageHandlers {
	return &MessageHandlers{
		PLCActionHandler:       mp.handlePLCActionMessage,
		RobotConnectionHandler: mp.handleRobotConnectionMessage,
		RobotFactsheetHandler:  mp.handleRobotFactsheetMessage,
	}
}

// handleRobotConnectionMessage processes robot connection status messages
func (mp *MessageProcessor) handleRobotConnectionMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📨 로봇 연결 상태 메시지 수신 - Topic: %s", msg.Topic())

	// Parse topic to get serial number
	serialNumber, err := parseRobotConnectionTopic(msg.Topic())
	if err != nil {
		log.Printf("❌ 토픽 파싱 실패: %v", err)
		return
	}

	// Try to parse as detailed AGV status first
	var agvStatus AGVDetailedStatus
	if err := json.Unmarshal(msg.Payload(), &agvStatus); err == nil {
		// Check if this looks like detailed AGV status (has required fields)
		if agvStatus.SerialNumber != "" && agvStatus.Manufacturer != "" && len(agvStatus.ActionStates) >= 0 {
			log.Printf("📊 AGV 상세 상태 메시지 수신 - Serial: %s", agvStatus.SerialNumber)
			mp.handleAGVDetailedStatus(&agvStatus, serialNumber)
			return
		}
	}

	// Fall back to parsing as simple connection message
	var connectionMsg RobotConnectionMessage
	if err := json.Unmarshal(msg.Payload(), &connectionMsg); err != nil {
		log.Printf("❌ JSON 파싱 실패 (연결 상태): %v", err)
		return
	}

	// Validate and update robot status
	if err := mp.validateAndUpdateRobotStatus(&connectionMsg, serialNumber); err != nil {
		log.Printf("❌ 로봇 상태 업데이트 실패: %v", err)
		return
	}

	log.Printf("✅ 로봇 상태 업데이트 완료 - Serial: %s, State: %s, HeaderID: %d",
		connectionMsg.SerialNumber, connectionMsg.ConnectionState, connectionMsg.HeaderID)
}

// handleAGVDetailedStatus processes detailed AGV status messages
func (mp *MessageProcessor) handleAGVDetailedStatus(agvStatus *AGVDetailedStatus, serialNumber string) {
	// Validate serial number consistency
	if agvStatus.SerialNumber != serialNumber {
		log.Printf("❌ AGV 상태 시리얼 번호 불일치 - Topic: %s, Message: %s", serialNumber, agvStatus.SerialNumber)
		return
	}

	// Check if this robot is in target list
	if !mp.robotManager.IsTargetRobot(serialNumber) {
		return // Silently ignore non-target robots
	}

	// Update robot detailed status
	mp.robotManager.UpdateRobotDetailedStatus(agvStatus)

	// Log essential status info
	log.Printf("📊 AGV 상세 상태 업데이트 - Serial: %s, 배터리: %.1f%%, 주행: %t",
		agvStatus.SerialNumber, agvStatus.BatteryState.BatteryLevel, agvStatus.Driving)
}

// validateAndUpdateRobotStatus validates and updates basic robot status
func (mp *MessageProcessor) validateAndUpdateRobotStatus(msg *RobotConnectionMessage, serialNumber string) error {
	// Validate message
	if msg.SerialNumber == "" || msg.Manufacturer == "" || msg.Version == "" {
		return fmt.Errorf("missing required fields in connection message")
	}

	// Validate serial number consistency
	if msg.SerialNumber != serialNumber {
		return fmt.Errorf("serial number mismatch - Topic: %s, Message: %s", serialNumber, msg.SerialNumber)
	}

	// Check if this robot is in target list
	if !mp.robotManager.IsTargetRobot(serialNumber) {
		return nil // Silently ignore non-target robots
	}

	// Update robot status
	mp.robotManager.UpdateRobotStatus(msg)
	return nil
}

// handleRobotFactsheetMessage processes robot factsheet response messages
func (mp *MessageProcessor) handleRobotFactsheetMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📋 로봇 Factsheet 응답 수신 - Topic: %s", msg.Topic())

	// Parse topic to get serial number
	serialNumber, _, err := parseRobotFactsheetTopic(msg.Topic())
	if err != nil {
		log.Printf("❌ Factsheet 토픽 파싱 실패: %v", err)
		return
	}

	// Check if this robot is in target list
	if !mp.robotManager.IsTargetRobot(serialNumber) {
		return // Silently ignore non-target robots
	}

	// Parse factsheet response
	var factsheetMsg FactsheetResponseMessage
	if err := json.Unmarshal(msg.Payload(), &factsheetMsg); err != nil {
		log.Printf("❌ Factsheet 응답 파싱 실패: %v", err)
		return
	}

	// Validate factsheet response
	if factsheetMsg.SerialNumber == "" || factsheetMsg.Version == "" {
		log.Printf("⚠️  유효하지 않은 Factsheet 응답 - Serial: %s", serialNumber)
		return
	}

	// Validate serial number consistency
	if factsheetMsg.SerialNumber != serialNumber {
		log.Printf("❌ Factsheet 시리얼 번호 불일치 - Topic: %s, Message: %s", serialNumber, factsheetMsg.SerialNumber)
		return
	}

	// Update robot factsheet status
	mp.robotManager.UpdateFactsheetReceived(serialNumber)

	// Log factsheet details
	log.Printf("📋 Factsheet 수신 완료 - Serial: %s, Manufacturer: %s, Actions: %d개",
		serialNumber, factsheetMsg.Manufacturer, len(factsheetMsg.ProtocolFeatures.AGVActions))
}

// handlePLCActionMessage processes PLC action messages from bridge/actions topic
func (mp *MessageProcessor) handlePLCActionMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📨 PLC 액션 메시지 수신 - Payload: %s", string(msg.Payload()))

	// Check MQTT connection
	if !mp.mqttClient.IsConnected() {
		log.Printf("❌ MQTT 클라이언트가 연결되지 않아 액션을 전송할 수 없습니다")
		return
	}

	// Parse and validate PLC action
	plcAction, err := ParsePLCActionMessage(msg.Payload())
	if err != nil {
		log.Printf("❌ PLC 액션 메시지 파싱 실패: %v", err)
		return
	}

	if err := ValidatePLCAction(plcAction); err != nil {
		log.Printf("❌ PLC 액션 검증 실패: %v", err)
		return
	}

	log.Printf("🚀 PLC 액션 처리 시작 - Action: %s, Target: %s", plcAction.Action, plcAction.SerialNumber)

	// Send action to target robot
	if err := mp.sendActionToRobot(plcAction, plcAction.SerialNumber); err != nil {
		log.Printf("❌ 로봇에 액션 전송 실패 - Serial: %s, Error: %v", plcAction.SerialNumber, err)
		return
	}

	log.Printf("✅ 로봇에 액션 전송 완료 - Serial: %s, Action: %s", plcAction.SerialNumber, plcAction.Action)
}

// sendActionToRobot sends action to a specific robot
func (mp *MessageProcessor) sendActionToRobot(plcAction *PLCActionMessage, serialNumber string) error {
	// Check if robot is online and is target robot
	if !mp.robotManager.IsTargetRobot(serialNumber) {
		return fmt.Errorf("robot %s is not in target list", serialNumber)
	}

	if !mp.robotManager.IsRobotOnline(serialNumber) {
		return fmt.Errorf("robot %s is not online", serialNumber)
	}

	// Convert PLC action to robot action
	robotAction, err := mp.actionHandler.ConvertPLCActionToRobotAction(plcAction, serialNumber)
	if err != nil {
		return fmt.Errorf("action conversion failed: %w", err)
	}

	// Convert to JSON
	payload, err := json.Marshal(robotAction)
	if err != nil {
		return fmt.Errorf("JSON marshaling failed: %w", err)
	}

	// Determine topic based on action type
	var topic string
	if plcAction.Action == "cancelOrder" {
		topic = buildRobotOrderTopic(serialNumber)
	} else {
		topic = buildRobotActionTopic(serialNumber)
	}

	// Publish to appropriate topic
	if err := mp.mqttClient.Publish(topic, payload); err != nil {
		return fmt.Errorf("MQTT publish failed: %w", err)
	}

	log.Printf("📤 로봇 액션 메시지 발행 - Topic: %s, HeaderID: %d, ActionType: %s",
		topic, robotAction.HeaderID, mp.getActionTypeForLogging(robotAction))

	return nil
}

// getActionTypeForLogging extracts action type for logging purposes
func (mp *MessageProcessor) getActionTypeForLogging(robotAction *RobotActionMessage) string {
	if len(robotAction.Actions) > 0 {
		return robotAction.Actions[0].ActionType
	}
	if len(robotAction.Nodes) > 0 && len(robotAction.Nodes[0].Actions) > 0 {
		return robotAction.Nodes[0].Actions[0].ActionType
	}
	return "unknown"
}

// SendFactsheetRequest sends factsheet request to a specific robot
func (mp *MessageProcessor) SendFactsheetRequest(serialNumber string, manufacturer string) error {
	// Create factsheet request
	factsheetRequest := mp.actionHandler.createFactsheetRequestAction(serialNumber, manufacturer)

	// Convert to JSON
	payload, err := json.Marshal(factsheetRequest)
	if err != nil {
		return fmt.Errorf("JSON marshaling failed: %w", err)
	}

	// Build topic and publish
	topic := buildRobotActionTopic(serialNumber)
	if err := mp.mqttClient.Publish(topic, payload); err != nil {
		return fmt.Errorf("MQTT publish failed: %w", err)
	}

	log.Printf("📤 Factsheet 요청 발행 - Topic: %s, HeaderID: %d",
		topic, factsheetRequest.HeaderID)

	return nil
}
