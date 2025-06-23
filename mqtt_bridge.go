package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// MQTTBridge coordinates all bridge components
type MQTTBridge struct {
	// Core components
	mqttClient       *MQTTClient
	robotManager     *RobotManager
	actionHandler    *ActionHandler
	messageProcessor *MessageProcessor
	statusMonitor    *RobotStatusMonitor
	config           *Config

	// Graceful shutdown
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	shutdownWG     sync.WaitGroup

	// Monitoring goroutines control
	statusMonitorStop   chan struct{}
	healthMonitorStop   chan struct{}
	detailedMonitorStop chan struct{}
}

// NewMQTTBridge creates a new MQTT bridge with all components
func NewMQTTBridge(config *Config) *MQTTBridge {
	// Create shutdown context
	ctx, cancel := context.WithCancel(context.Background())

	// Create core components
	robotManager := NewRobotManager(config.App.TargetRobotSerials)
	actionHandler := NewActionHandler()

	// Create MQTT client (without handlers initially)
	mqttClient := NewMQTTClient(&config.MQTT, nil)

	// Create message processor
	messageProcessor := NewMessageProcessor(mqttClient, robotManager, actionHandler, config)

	// Set message handlers for MQTT client
	mqttClient.handlers = messageProcessor.GetMessageHandlers()

	// Create status monitor
	statusMonitor := NewRobotStatusMonitor(robotManager, messageProcessor, config)

	return &MQTTBridge{
		mqttClient:          mqttClient,
		robotManager:        robotManager,
		actionHandler:       actionHandler,
		messageProcessor:    messageProcessor,
		statusMonitor:       statusMonitor,
		config:              config,
		shutdownCtx:         ctx,
		shutdownCancel:      cancel,
		statusMonitorStop:   make(chan struct{}),
		healthMonitorStop:   make(chan struct{}),
		detailedMonitorStop: make(chan struct{}),
	}
}

// Start initializes and starts the MQTT bridge
func (mb *MQTTBridge) Start() error {
	log.Printf("🚀 MQTT 브릿지 시작 중...")
	log.Printf("📋 설정 정보 - Broker: %s, ClientID: %s, ConnectTimeout: %ds, MaxReconnectAttempts: %d",
		mb.config.MQTT.BrokerURL, mb.config.MQTT.ClientID, mb.config.MQTT.ConnectTimeout, mb.config.MQTT.MaxReconnectAttempts)

	// Connect to MQTT broker
	if err := mb.mqttClient.Connect(); err != nil {
		return fmt.Errorf("MQTT 연결 실패: %w", err)
	}

	// Start monitoring components
	mb.startMonitoring()

	log.Printf("✅ MQTT 브릿지 시작 완료")
	return nil
}

// startMonitoring starts all monitoring goroutines
func (mb *MQTTBridge) startMonitoring() {
	// Start MQTT connection monitoring
	mb.mqttClient.StartConnectionMonitor()

	// Start status monitoring goroutine
	mb.shutdownWG.Add(1)
	go func() {
		defer mb.shutdownWG.Done()
		mb.runStatusMonitoring()
	}()

	// Start health check goroutine
	mb.shutdownWG.Add(1)
	go func() {
		defer mb.shutdownWG.Done()
		mb.runHealthMonitoring()
	}()

	// Start detailed status monitoring goroutine
	mb.shutdownWG.Add(1)
	go func() {
		defer mb.shutdownWG.Done()
		mb.runDetailedStatusMonitoring()
	}()
}

// runStatusMonitoring runs the main status monitoring loop
func (mb *MQTTBridge) runStatusMonitoring() {
	ticker := time.NewTicker(time.Duration(mb.config.App.StatusIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Print connection status
			status := mb.mqttClient.GetConnectionStatus()
			log.Printf("MQTT 연결: %s", status)

			// Print robot status summary
			mb.statusMonitor.PrintStatusSummary()

			// Check for alerts
			if status != Connected {
				log.Printf("⚠️  MQTT 연결 문제 - 상태: %s", status)
			}

			// Check battery levels
			mb.statusMonitor.CheckBatteryLevels()

		case <-mb.statusMonitorStop:
			return
		case <-mb.shutdownCtx.Done():
			return
		}
	}
}

// runHealthMonitoring runs the connection health monitoring loop
func (mb *MQTTBridge) runHealthMonitoring() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	consecutiveFailures := 0
	const maxFailures = 3

	for {
		select {
		case <-ticker.C:
			if !mb.mqttClient.IsConnected() {
				consecutiveFailures++
				status := mb.mqttClient.GetConnectionStatus()

				if consecutiveFailures >= maxFailures {
					log.Printf("🚨 심각: MQTT 연결 실패가 %d회 연속 발생 - 상태: %s",
						consecutiveFailures, status)
				} else {
					log.Printf("⚠️  MQTT 연결 확인 필요 (%d/%d) - 상태: %s",
						consecutiveFailures, maxFailures, status)
				}
			} else {
				if consecutiveFailures > 0 {
					log.Printf("✅ MQTT 연결 복구됨 (이전 실패: %d회)", consecutiveFailures)
				}
				consecutiveFailures = 0
			}

		case <-mb.healthMonitorStop:
			return
		case <-mb.shutdownCtx.Done():
			return
		}
	}
}

// runDetailedStatusMonitoring runs the detailed status monitoring loop
func (mb *MQTTBridge) runDetailedStatusMonitoring() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mb.statusMonitor.PrintDetailedStatusReport()

		case <-mb.detailedMonitorStop:
			return
		case <-mb.shutdownCtx.Done():
			return
		}
	}
}

// Stop gracefully shuts down the MQTT bridge
func (mb *MQTTBridge) Stop() {
	log.Printf("🛑 MQTT 브릿지 종료 중...")

	// Signal shutdown to all components
	mb.shutdownCancel()

	// Stop monitoring goroutines
	close(mb.statusMonitorStop)
	close(mb.healthMonitorStop)
	close(mb.detailedMonitorStop)

	// Stop MQTT client
	mb.mqttClient.Stop()

	// Wait for all goroutines to finish
	mb.shutdownWG.Wait()

	log.Printf("✅ MQTT 브릿지 종료 완료")
}

// IsConnected checks if the MQTT client is connected
func (mb *MQTTBridge) IsConnected() bool {
	return mb.mqttClient.IsConnected()
}

// GetConnectionStatus returns the current MQTT connection status
func (mb *MQTTBridge) GetConnectionStatus() ConnectionStatus {
	return mb.mqttClient.GetConnectionStatus()
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

// GetMQTTClient returns the MQTT client instance
func (mb *MQTTBridge) GetMQTTClient() *MQTTClient {
	return mb.mqttClient
}

// GetMessageProcessor returns the message processor instance
func (mb *MQTTBridge) GetMessageProcessor() *MessageProcessor {
	return mb.messageProcessor
}

// GetStatusMonitor returns the status monitor instance
func (mb *MQTTBridge) GetStatusMonitor() *RobotStatusMonitor {
	return mb.statusMonitor
}

// SendActionToRobot sends an action to a specific robot (public interface)
func (mb *MQTTBridge) SendActionToRobot(action *PLCActionMessage, serialNumber string) error {
	return mb.messageProcessor.sendActionToRobot(action, serialNumber)
}

// SendFactsheetRequest sends a factsheet request to a specific robot (public interface)
func (mb *MQTTBridge) SendFactsheetRequest(serialNumber string, manufacturer string) error {
	return mb.messageProcessor.SendFactsheetRequest(serialNumber, manufacturer)
}

// GetBridgeStatus returns overall bridge status information
func (mb *MQTTBridge) GetBridgeStatus() BridgeStatus {
	mqttStatus := mb.mqttClient.GetConnectionStatus()
	onlineRobots := mb.robotManager.GetOnlineRobots()
	allRobots := mb.robotManager.GetAllRobots()
	targetRobotCount := mb.robotManager.GetTargetRobotCount()
	reconnectCount := mb.mqttClient.GetReconnectCount()

	return BridgeStatus{
		MQTTConnectionStatus: mqttStatus,
		MQTTReconnectCount:   reconnectCount,
		TotalRobots:          len(allRobots),
		OnlineRobots:         len(onlineRobots),
		TargetRobotCount:     targetRobotCount,
		LastStatusUpdate:     time.Now(),
	}
}

// BridgeStatus represents the overall status of the bridge
type BridgeStatus struct {
	MQTTConnectionStatus ConnectionStatus `json:"mqttConnectionStatus"`
	MQTTReconnectCount   int32            `json:"mqttReconnectCount"`
	TotalRobots          int              `json:"totalRobots"`
	OnlineRobots         int              `json:"onlineRobots"`
	TargetRobotCount     int              `json:"targetRobotCount"`
	LastStatusUpdate     time.Time        `json:"lastStatusUpdate"`
}
