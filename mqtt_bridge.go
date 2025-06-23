package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// ConnectionStatus represents the connection status
type ConnectionStatus int

const (
	Disconnected ConnectionStatus = iota
	Connecting
	Connected
	ConnectionLost
	ConnectionFailed
)

func (cs ConnectionStatus) String() string {
	switch cs {
	case Disconnected:
		return "DISCONNECTED"
	case Connecting:
		return "CONNECTING"
	case Connected:
		return "CONNECTED"
	case ConnectionLost:
		return "CONNECTION_LOST"
	case ConnectionFailed:
		return "CONNECTION_FAILED"
	default:
		return "UNKNOWN"
	}
}

// MQTTBridge handles MQTT connections and message routing with single client
type MQTTBridge struct {
	client        mqtt.Client
	robotManager  *RobotManager
	actionHandler *ActionHandler
	config        *Config

	// Connection status tracking
	status      ConnectionStatus
	statusMutex sync.RWMutex

	// Reconnection tracking
	reconnectCount int32

	// Graceful shutdown
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	shutdownWG     sync.WaitGroup

	// Connection monitoring
	connectionMonitorStop chan struct{}
}

// NewMQTTBridge creates a new MQTT bridge with single client
func NewMQTTBridge(config *Config) *MQTTBridge {
	robotManager := NewRobotManager(config.App.TargetRobotSerials)
	actionHandler := NewActionHandler()

	ctx, cancel := context.WithCancel(context.Background())

	bridge := &MQTTBridge{
		robotManager:          robotManager,
		actionHandler:         actionHandler,
		config:                config,
		status:                Disconnected,
		shutdownCtx:           ctx,
		shutdownCancel:        cancel,
		connectionMonitorStop: make(chan struct{}),
	}

	// Set status change callback for automatic init position
	robotManager.SetStatusChangeCallback(bridge.handleRobotStatusChange)

	// Create single MQTT Client
	bridge.client = bridge.createMQTTClient()

	return bridge
}

// createMQTTClient creates a single MQTT client for the bridge
func (mb *MQTTBridge) createMQTTClient() mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mb.config.MQTT.BrokerURL)
	opts.SetClientID(mb.config.MQTT.ClientID)

	if mb.config.MQTT.Username != "" {
		opts.SetUsername(mb.config.MQTT.Username)
		opts.SetPassword(mb.config.MQTT.Password)
	}

	opts.SetKeepAlive(time.Duration(mb.config.MQTT.KeepAlive) * time.Second)
	opts.SetConnectTimeout(time.Duration(mb.config.MQTT.ConnectTimeout) * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetCleanSession(mb.config.MQTT.CleanSession)
	opts.SetMaxReconnectInterval(time.Duration(mb.config.MQTT.MaxReconnectDelay) * time.Second)

	// Connection handlers
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		mb.updateStatus(Connected)
		reconnectCount := atomic.LoadInt32(&mb.reconnectCount)
		if reconnectCount > 0 {
			log.Printf("✅ MQTT 재연결 성공 - Broker: %s (재연결 횟수: %d)", mb.config.MQTT.BrokerURL, reconnectCount)
		} else {
			log.Printf("✅ MQTT 브릿지 연결됨 - Broker: %s, ClientID: %s", mb.config.MQTT.BrokerURL, mb.config.MQTT.ClientID)
		}

		// Subscribe to all topics on (re)connection
		mb.subscribeToTopics()
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		mb.updateStatus(ConnectionLost)
		log.Printf("❌ MQTT 연결 끊어짐 - Error: %v", err)
		atomic.AddInt32(&mb.reconnectCount, 1)
	})

	opts.SetReconnectingHandler(func(client mqtt.Client, opts *mqtt.ClientOptions) {
		mb.updateStatus(Connecting)
		reconnectCount := atomic.LoadInt32(&mb.reconnectCount)
		log.Printf("🔄 MQTT 재연결 시도 중... (시도 횟수: %d)", reconnectCount)
	})

	return mqtt.NewClient(opts)
}

// updateStatus updates connection status
func (mb *MQTTBridge) updateStatus(status ConnectionStatus) {
	mb.statusMutex.Lock()
	defer mb.statusMutex.Unlock()
	mb.status = status
}

// GetConnectionStatus returns current connection status
func (mb *MQTTBridge) GetConnectionStatus() ConnectionStatus {
	mb.statusMutex.RLock()
	defer mb.statusMutex.RUnlock()
	return mb.status
}

// IsConnected checks if the client is connected
func (mb *MQTTBridge) IsConnected() bool {
	return mb.GetConnectionStatus() == Connected && mb.client.IsConnected()
}

// connectWithRetry attempts to connect with retry logic
func (mb *MQTTBridge) connectWithRetry() error {
	maxAttempts := mb.config.MQTT.MaxReconnectAttempts
	connectTimeout := time.Duration(mb.config.MQTT.ConnectTimeout) * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("🔌 MQTT 연결 시도 중... (%d/%d) - Broker: %s",
			attempt, maxAttempts, mb.config.MQTT.BrokerURL)

		// Attempt connection
		token := mb.client.Connect()

		// Wait for connection with timeout
		if token.WaitTimeout(connectTimeout) && token.Error() == nil {
			log.Printf("✅ MQTT 연결 성공")
			return nil
		}

		// Log connection error
		if token.Error() != nil {
			log.Printf("❌ MQTT 연결 실패 (%d/%d) - Error: %v",
				attempt, maxAttempts, token.Error())
		} else {
			log.Printf("❌ MQTT 연결 타임아웃 (%d/%d) - Timeout: %v",
				attempt, maxAttempts, connectTimeout)
		}

		// Wait before retry (except for last attempt)
		if attempt < maxAttempts {
			retryDelay := time.Duration(mb.config.MQTT.ReconnectDelay) * time.Second
			log.Printf("⏳ %d초 후 재시도...", mb.config.MQTT.ReconnectDelay)

			select {
			case <-time.After(retryDelay):
				// Continue to next attempt
			case <-mb.shutdownCtx.Done():
				return fmt.Errorf("연결 시도 중 종료 요청됨")
			}
		}
	}

	return fmt.Errorf("MQTT 연결 실패 - 최대 재시도 횟수 초과 (%d번)", maxAttempts)
}

// startConnectionMonitor monitors connection status and logs periodically
func (mb *MQTTBridge) startConnectionMonitor() {
	mb.shutdownWG.Add(1)
	go func() {
		defer mb.shutdownWG.Done()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				status := mb.GetConnectionStatus()
				reconnects := atomic.LoadInt32(&mb.reconnectCount)

				log.Printf("📊 MQTT 연결 상태: %s (재연결: %d회)", status, reconnects)

				// Alert if connection is down
				if status != Connected {
					log.Printf("⚠️  MQTT 연결 이상 - 상태: %s", status)
				}

			case <-mb.connectionMonitorStop:
				return
			case <-mb.shutdownCtx.Done():
				return
			}
		}
	}()
}

// subscribeToTopics subscribes to all required topics
func (mb *MQTTBridge) subscribeToTopics() {
	if !mb.client.IsConnected() {
		log.Printf("❌ MQTT 클라이언트가 연결되지 않아 토픽 구독 불가")
		return
	}

	// Subscribe to PLC action messages
	actionTopic := "bridge/actions"
	token := mb.client.Subscribe(actionTopic, mb.config.MQTT.QoS, mb.handlePLCActionMessage)
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		log.Printf("✅ PLC 액션 토픽 구독 완료: %s", actionTopic)
	} else {
		log.Printf("❌ PLC 액션 토픽 구독 실패: %v", token.Error())
	}

	// Subscribe to robot connection status messages
	connectionTopic := "meili/v2/Roboligent/+/connection"
	token = mb.client.Subscribe(connectionTopic, mb.config.MQTT.QoS, mb.handleRobotConnectionMessage)
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		log.Printf("✅ 로봇 연결 상태 토픽 구독 완료: %s", connectionTopic)
	} else {
		log.Printf("❌ 로봇 연결 상태 토픽 구독 실패: %v", token.Error())
	}

	// Subscribe to robot factsheet responses
	factsheetTopic := "meili/v2/+/+/factsheet"
	token = mb.client.Subscribe(factsheetTopic, mb.config.MQTT.QoS, mb.handleRobotFactsheetMessage)
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		log.Printf("✅ 로봇 Factsheet 토픽 구독 완료: %s", factsheetTopic)
	} else {
		log.Printf("❌ 로봇 Factsheet 토픽 구독 실패: %v", token.Error())
	}
}

// handleRobotConnectionMessage processes robot connection status messages
func (mb *MQTTBridge) handleRobotConnectionMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📨 로봇 연결 상태 메시지 수신 - Topic: %s", msg.Topic())

	// Parse topic to get serial number
	serialNumber, err := parseRobotConnectionTopic(msg.Topic())
	if err != nil {
		log.Printf("❌ 토픽 파싱 실패: %v", err)
		return
	}

	// Parse JSON message
	var connectionMsg RobotConnectionMessage
	if err := json.Unmarshal(msg.Payload(), &connectionMsg); err != nil {
		log.Printf("❌ JSON 파싱 실패: %v", err)
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
	mb.robotManager.UpdateRobotStatus(&connectionMsg)

	// Log message details
	log.Printf("✅ 로봇 상태 업데이트 완료 - Serial: %s, State: %s, HeaderID: %d",
		connectionMsg.SerialNumber, connectionMsg.ConnectionState, connectionMsg.HeaderID)
}

// handleRobotFactsheetMessage processes robot factsheet response messages
func (mb *MQTTBridge) handleRobotFactsheetMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📋 로봇 Factsheet 응답 수신 - Topic: %s", msg.Topic())

	// Parse topic to get serial number and manufacturer
	serialNumber, _, err := parseRobotFactsheetTopic(msg.Topic())
	if err != nil {
		log.Printf("❌ Factsheet 토픽 파싱 실패: %v", err)
		return
	}

	// Check if this robot is in target list
	if !mb.robotManager.IsTargetRobot(serialNumber) {
		log.Printf("⚠️  관리 대상이 아닌 로봇 Factsheet 무시 - Serial: %s", serialNumber)
		return
	}

	// Parse as factsheet response (로봇이 보낸 응답이므로 바로 파싱)
	var factsheetMsg FactsheetResponseMessage
	if err := json.Unmarshal(msg.Payload(), &factsheetMsg); err != nil {
		log.Printf("❌ Factsheet 응답 파싱 실패: %v", err)
		return
	}

	// Validate that this is actually a factsheet response (should have required fields)
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
	mb.robotManager.UpdateFactsheetReceived(serialNumber)

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
func (mb *MQTTBridge) handlePLCActionMessage(client mqtt.Client, msg mqtt.Message) {
	log.Printf("📨 PLC 액션 메시지 수신 - Topic: %s, Payload: %s", msg.Topic(), string(msg.Payload()))

	// Check if we can send to robots
	if !mb.client.IsConnected() {
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
	var targetRobots []string
	if plcAction.SerialNumber != "" {
		// Send to specific robot
		if mb.robotManager.IsRobotOnline(plcAction.SerialNumber) {
			// Check if the specified robot is in target list
			if !mb.robotManager.IsTargetRobot(plcAction.SerialNumber) {
				log.Printf("⚠️  지정된 로봇이 관리 대상이 아닙니다 - Serial: %s", plcAction.SerialNumber)
				return
			}
			targetRobots = []string{plcAction.SerialNumber}
		} else {
			log.Printf("⚠️  지정된 로봇이 오프라인 상태입니다 - Serial: %s", plcAction.SerialNumber)
			return
		}
	} else {
		// Send to all online target robots
		onlineTargetRobots := []string{}
		for _, serial := range mb.robotManager.GetOnlineRobots() {
			if mb.robotManager.IsTargetRobot(serial) {
				onlineTargetRobots = append(onlineTargetRobots, serial)
			}
		}
		targetRobots = onlineTargetRobots
	}

	if len(targetRobots) == 0 {
		log.Printf("⚠️  액션을 전송할 온라인 로봇이 없습니다")
		return
	}

	// Send action to each target robot
	successCount := 0
	for _, serialNumber := range targetRobots {
		if err := mb.sendActionToRobot(plcAction, serialNumber); err != nil {
			log.Printf("❌ 로봇에 액션 전송 실패 - Serial: %s, Error: %v", serialNumber, err)
		} else {
			log.Printf("✅ 로봇에 액션 전송 완료 - Serial: %s, Action: %s", serialNumber, plcAction.Action)
			successCount++
		}
	}

	log.Printf("📊 PLC 액션 처리 완료 - 성공: %d/%d", successCount, len(targetRobots))
}

// sendActionToRobot sends action to a specific robot
func (mb *MQTTBridge) sendActionToRobot(plcAction *PLCActionMessage, serialNumber string) error {
	// Check client connection
	if !mb.client.IsConnected() {
		return fmt.Errorf("MQTT 클라이언트가 연결되지 않음")
	}

	// Convert PLC action to robot action
	robotAction, err := mb.actionHandler.ConvertPLCActionToRobotAction(plcAction, serialNumber)
	if err != nil {
		return fmt.Errorf("액션 변환 실패: %w", err)
	}

	// Validate robot action message
	if err := validateRobotActionMessage(robotAction); err != nil {
		return fmt.Errorf("로봇 액션 메시지 검증 실패: %w", err)
	}

	// Convert to JSON
	payload, err := json.Marshal(robotAction)
	if err != nil {
		return fmt.Errorf("JSON 마샬링 실패: %w", err)
	}

	// Build topic
	topic := buildRobotActionTopic(serialNumber)

	// Publish to robot with timeout check
	token := mb.client.Publish(topic, mb.config.MQTT.QoS, false, payload)

	// Wait for publish completion with timeout
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("MQTT 발행 타임아웃")
	}

	if token.Error() != nil {
		return fmt.Errorf("MQTT 발행 실패: %w", token.Error())
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

// sendFactsheetRequest sends factsheet request to a specific robot
func (mb *MQTTBridge) sendFactsheetRequest(serialNumber string, manufacturer string) error {
	// Check client connection
	if !mb.client.IsConnected() {
		return fmt.Errorf("MQTT 클라이언트가 연결되지 않음")
	}

	// Create factsheet request (using standard robot action format)
	factsheetRequest := mb.actionHandler.createFactsheetRequestAction(serialNumber, manufacturer)

	// Convert to JSON
	payload, err := json.Marshal(factsheetRequest)
	if err != nil {
		return fmt.Errorf("JSON 마샬링 실패: %w", err)
	}

	// Build topic - use instantActions topic, not factsheet topic
	topic := buildRobotActionTopic(serialNumber)

	// Publish factsheet request with timeout check
	token := mb.client.Publish(topic, mb.config.MQTT.QoS, false, payload)

	// Wait for publish completion with timeout
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("MQTT 발행 타임아웃")
	}

	if token.Error() != nil {
		return fmt.Errorf("MQTT 발행 실패: %w", token.Error())
	}

	log.Printf("📤 로봇 Factsheet 요청 발행 - Topic: %s, HeaderID: %d, ActionID: %s",
		topic, factsheetRequest.HeaderID, factsheetRequest.Actions[0].ActionID)

	return nil
}

// handleRobotStatusChange handles robot status changes and sends init command when robot comes online
func (mb *MQTTBridge) handleRobotStatusChange(serialNumber string, oldState, newState ConnectionState) {
	// Check if auto init is enabled
	if !mb.config.App.AutoInitOnConnect {
		return
	}

	// Check if robot changed from non-ONLINE to ONLINE
	if oldState != Online && newState == Online {
		log.Printf("🤖 로봇 온라인 감지 - 자동 위치 초기화 시작: %s", serialNumber)

		// Create init action for the robot
		initAction := &PLCActionMessage{
			Action:       "init",
			SerialNumber: serialNumber,
		}

		// Send init action to the robot (with configurable delay)
		go func() {
			// Wait for robot to fully initialize
			delayDuration := time.Duration(mb.config.App.AutoInitDelaySec) * time.Second
			log.Printf("⏳ 자동 초기화 대기 중 (%ds): %s", mb.config.App.AutoInitDelaySec, serialNumber)
			time.Sleep(delayDuration)

			// Check if robot is still online and MQTT is connected
			if !mb.IsConnected() {
				log.Printf("⚠️  MQTT 연결 없음 - 자동 초기화 취소: %s", serialNumber)
				return
			}

			if !mb.robotManager.IsRobotOnline(serialNumber) {
				log.Printf("⚠️  로봇 오프라인 됨 - 자동 초기화 취소: %s", serialNumber)
				return
			}

			// Send init action
			if err := mb.sendActionToRobot(initAction, serialNumber); err != nil {
				log.Printf("❌ 자동 위치 초기화 실패 - Serial: %s, Error: %v", serialNumber, err)
				return
			}

			log.Printf("✅ 자동 위치 초기화 완료 - Serial: %s", serialNumber)

			// After successful init, request factsheet if enabled
			if mb.config.App.AutoFactsheetRequest {
				if robot, exists := mb.robotManager.GetRobotStatus(serialNumber); exists {
					// Wait a bit more for init to complete before requesting factsheet
					time.Sleep(1 * time.Second)

					log.Printf("📋 Factsheet 요청 시작 - Serial: %s", serialNumber)
					if err := mb.sendFactsheetRequest(serialNumber, robot.Manufacturer); err != nil {
						log.Printf("❌ Factsheet 요청 실패 - Serial: %s, Error: %v", serialNumber, err)
					} else {
						log.Printf("✅ Factsheet 요청 완료 - Serial: %s", serialNumber)
					}
				}
			}
		}()
	}
}

// Start initializes and starts the MQTT bridge
func (mb *MQTTBridge) Start() error {
	log.Printf("🚀 MQTT 브릿지 시작 중...")
	log.Printf("📋 설정 정보 - Broker: %s, ClientID: %s, ConnectTimeout: %ds, MaxReconnectAttempts: %d",
		mb.config.MQTT.BrokerURL, mb.config.MQTT.ClientID, mb.config.MQTT.ConnectTimeout, mb.config.MQTT.MaxReconnectAttempts)

	// Connect to MQTT broker
	if err := mb.connectWithRetry(); err != nil {
		return fmt.Errorf("MQTT 연결 실패: %w", err)
	}

	// Start connection monitoring
	mb.startConnectionMonitor()

	log.Printf("✅ MQTT 브릿지 시작 완료")
	return nil
}

// Stop gracefully disconnects the MQTT client
func (mb *MQTTBridge) Stop() {
	log.Printf("🛑 MQTT 브릿지 종료 중...")

	// Signal shutdown
	mb.shutdownCancel()

	// Stop connection monitor
	close(mb.connectionMonitorStop)

	// Disconnect client
	if mb.client.IsConnected() {
		mb.client.Disconnect(250)
		log.Printf("✅ MQTT 클라이언트 연결 해제됨")
	}

	// Wait for goroutines to finish
	mb.shutdownWG.Wait()

	log.Printf("✅ MQTT 브릿지 종료 완료")
}

// GetRobotManager returns the robot manager instance
func (mb *MQTTBridge) GetRobotManager() *RobotManager {
	return mb.robotManager
}

// GetActionHandler returns the action handler instance
func (mb *MQTTBridge) GetActionHandler() *ActionHandler {
	return mb.actionHandler
}

// GetConfig returns the bridge configuration
func (mb *MQTTBridge) GetConfig() *Config {
	return mb.config
}
