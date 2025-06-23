package main

import (
	"encoding/json"
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
		if agvStatus.SerialNumber != "" && agvStatus.Manufacturer != "" {
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

	// Validate message
	if err := validateRobotMessage(&connectionMsg); err != nil {
		log.Printf("❌ 메시지 검증 실패: %v", err)
		return
	}

	// Validate serial number consistency
	if connectionMsg.SerialNumber != serialNumber {
		log.Printf("❌ 시리얼 번호 불일치 - Topic: %s, Message: %s", serialNumber, connectionMsg.SerialNumber)
		return
	}

	// Update robot status
	mp.robotManager.UpdateRobotStatus(&connectionMsg)

	// Log message details
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
		log.Printf("⚠️  관리 대상이 아닌 로봇 AGV 상태 무시 - Serial: %s", serialNumber)
		return
	}

	// Update robot detailed status
	mp.robotManager.UpdateRobotDetailedStatus(agvStatus)

	// Log detailed status
	log.Printf("📊 AGV 상세 상태 업데이트 완료 - Serial: %s", agvStatus.SerialNumber)
	log.Printf("   - 위치: (%.2f, %.2f, %.2f°)", agvStatus.AGVPosition.X, agvStatus.AGVPosition.Y, agvStatus.AGVPosition.Theta*180/3.14159)
	log.Printf("   - 배터리: %.1f%% (충전중: %t)", agvStatus.BatteryState.BatteryCharge, agvStatus.BatteryState.Charging)
	log.Printf("   - 주행중: %t, 일시정지: %t", agvStatus.Driving, agvStatus.Paused)
	log.Printf("   - 운영모드: %s", agvStatus.OperatingMode)

	if agvStatus.OrderID != "" {
		log.Printf("   - 현재 주문: %s (업데이트: %d)", agvStatus.OrderID, agvStatus.OrderUpdateID)
	}

	if len(agvStatus.ActionStates) > 0 {
		log.Printf("   - 실행 중인 액션: %d개", len(agvStatus.ActionStates))
		for i, action := range agvStatus.ActionStates {
			log.Printf("     %d. %s (%s)", i+1, action.ActionType, action.ActionStatus)
		}
	}

	if len(agvStatus.Errors) > 0 {
		log.Printf("   - ⚠️  에러: %d개", len(agvStatus.Errors))
	}
}

// handleRobotFactsheetMessage processes robot factsheet response messages
func (mp *MessageProcessor) handleRobotFactsheetMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📋 로봇 Factsheet 응답 수신 - Topic: %s", msg.Topic())

	// Parse topic to get serial number and manufacturer
	serialNumber, _, err := parseRobotFactsheetTopic(msg.Topic())
	if err != nil {
		log.Printf("❌ Factsheet 토픽 파싱 실패: %v", err)
		return
	}

	// Check if this robot is in target list
	if !mp.robotManager.IsTargetRobot(serialNumber) {
		log.Printf("⚠️  관리 대상이 아닌 로봇 Factsheet 무시 - Serial: %s", serialNumber)
		return
	}

	// Parse as factsheet response
	var factsheetMsg FactsheetResponseMessage
	if err := json.Unmarshal(msg.Payload(), &factsheetMsg); err != nil {
		log.Printf("❌ Factsheet 응답 파싱 실패: %v", err)
		return
	}

	// Validate that this is actually a factsheet response
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
	log.Printf("📋 로봇 Factsheet 상세 정보 - Serial: %s", serialNumber)
	log.Printf("   - Manufacturer: %s", factsheetMsg.Manufacturer)
	log.Printf("   - Version: %s", factsheetMsg.Version)
	log.Printf("   - Series: %s (%s)", factsheetMsg.TypeSpecification.SeriesName, factsheetMsg.TypeSpecification.AGVClass)
	log.Printf("   - Max Speed: %.1f m/s", factsheetMsg.PhysicalParameters.SpeedMax)
	log.Printf("   - Dimensions: %.1f x %.1f x %.1f m",
		factsheetMsg.PhysicalParameters.Length,
		factsheetMsg.PhysicalParameters.Width,
		factsheetMsg.PhysicalParameters.HeightMax)
	log.Printf("   - Available Actions: %d", len(factsheetMsg.ProtocolFeatures.AGVActions))

	for i, action := range factsheetMsg.ProtocolFeatures.AGVActions {
		log.Printf("     %d. %s (%d parameters)", i+1, action.ActionType, len(action.ActionParameters))
	}
}

// handlePLCActionMessage processes PLC action messages from bridge/actions topic
func (mp *MessageProcessor) handlePLCActionMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📨 PLC 액션 메시지 수신 - Topic: %s, Payload: %s", msg.Topic(), string(msg.Payload()))

	// Check if we can send to robots
	if !mp.mqttClient.IsConnected() {
		log.Printf("❌ MQTT 클라이언트가 연결되지 않아 액션을 전송할 수 없습니다")
		return
	}

	// Parse PLC action message
	plcAction, err := ParsePLCActionMessage(msg.Payload())
	if err != nil {
		log.Printf("❌ PLC 액션 메시지 파싱 실패: %v", err)
		return
	}

	// Validate PLC action
	if err := ValidatePLCAction(plcAction); err != nil {
		log.Printf("❌ PLC 액션 검증 실패: %v", err)
		return
	}

	log.Printf("🚀 PLC 액션 처리 시작 - Action: %s", plcAction.Action)

	// Determine target robots
	targetRobots := mp.determineTargetRobots(plcAction)
	if len(targetRobots) == 0 {
		log.Printf("⚠️  액션을 전송할 온라인 로봇이 없습니다")
		return
	}

	// Send action to each target robot
	successCount := 0
	for _, serialNumber := range targetRobots {
		if err := mp.sendActionToRobot(plcAction, serialNumber); err != nil {
			log.Printf("❌ 로봇에 액션 전송 실패 - Serial: %s, Error: %v", serialNumber, err)
		} else {
			log.Printf("✅ 로봇에 액션 전송 완료 - Serial: %s, Action: %s", serialNumber, plcAction.Action)
			successCount++
		}
	}

	log.Printf("📊 PLC 액션 처리 완료 - 성공: %d/%d", successCount, len(targetRobots))
}

// determineTargetRobots determines which robots should receive the action
func (mp *MessageProcessor) determineTargetRobots(plcAction *PLCActionMessage) []string {
	if plcAction.SerialNumber != "" {
		// Send to specific robot
		if mp.robotManager.IsRobotOnline(plcAction.SerialNumber) {
			// Check if the specified robot is in target list
			if !mp.robotManager.IsTargetRobot(plcAction.SerialNumber) {
				log.Printf("⚠️  지정된 로봇이 관리 대상이 아닙니다 - Serial: %s", plcAction.SerialNumber)
				return []string{}
			}
			return []string{plcAction.SerialNumber}
		} else {
			log.Printf("⚠️  지정된 로봇이 오프라인 상태입니다 - Serial: %s", plcAction.SerialNumber)
			return []string{}
		}
	}

	// Send to all online target robots
	var onlineTargetRobots []string
	for _, serial := range mp.robotManager.GetOnlineRobots() {
		if mp.robotManager.IsTargetRobot(serial) {
			onlineTargetRobots = append(onlineTargetRobots, serial)
		}
	}
	return onlineTargetRobots
}

// sendActionToRobot sends action to a specific robot
func (mp *MessageProcessor) sendActionToRobot(plcAction *PLCActionMessage, serialNumber string) error {
	// Convert PLC action to robot action
	robotAction, err := mp.actionHandler.ConvertPLCActionToRobotAction(plcAction, serialNumber)
	if err != nil {
		return err
	}

	// Validate robot action message
	if err := validateRobotActionMessage(robotAction); err != nil {
		return err
	}

	// Convert to JSON
	payload, err := json.Marshal(robotAction)
	if err != nil {
		return err
	}

	// Build topic and publish
	topic := buildRobotActionTopic(serialNumber)
	if err := mp.mqttClient.Publish(topic, payload); err != nil {
		return err
	}

	log.Printf("📤 로봇 액션 메시지 발행 - Topic: %s, HeaderID: %d",
		topic, robotAction.HeaderID)

	// Log action details based on message type
	if len(robotAction.Actions) > 0 {
		log.Printf("   Action Type: %s, ActionID: %s",
			robotAction.Actions[0].ActionType, robotAction.Actions[0].ActionID)
	} else if len(robotAction.Nodes) > 0 {
		log.Printf("   Order Type: OrderID: %s, Nodes: %d",
			robotAction.OrderID, len(robotAction.Nodes))
		for i, node := range robotAction.Nodes {
			if len(node.Actions) > 0 {
				log.Printf("   Node[%d] Actions: %s", i, node.Actions[0].ActionType)
			}
		}
	}

	return nil
}

// SendFactsheetRequest sends factsheet request to a specific robot
func (mp *MessageProcessor) SendFactsheetRequest(serialNumber string, manufacturer string) error {
	// Create factsheet request
	factsheetRequest := mp.actionHandler.createFactsheetRequestAction(serialNumber, manufacturer)

	// Convert to JSON
	payload, err := json.Marshal(factsheetRequest)
	if err != nil {
		return err
	}

	// Build topic and publish
	topic := buildRobotActionTopic(serialNumber)
	if err := mp.mqttClient.Publish(topic, payload); err != nil {
		return err
	}

	log.Printf("📤 로봇 Factsheet 요청 발행 - Topic: %s, HeaderID: %d, ActionID: %s",
		topic, factsheetRequest.HeaderID, factsheetRequest.Actions[0].ActionID)

	return nil
}
