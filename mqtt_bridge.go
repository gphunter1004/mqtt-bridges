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

	// Unified monitoring control
	statusMonitorStop chan struct{}
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
		mqttClient:        mqttClient,
		robotManager:      robotManager,
		actionHandler:     actionHandler,
		messageProcessor:  messageProcessor,
		statusMonitor:     statusMonitor,
		config:            config,
		shutdownCtx:       ctx,
		shutdownCancel:    cancel,
		statusMonitorStop: make(chan struct{}),
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
	// Start unified monitoring goroutine (combines status + health)
	mb.shutdownWG.Add(1)
	go func() {
		defer mb.shutdownWG.Done()
		mb.runUnifiedMonitoring()
	}()
}

// runUnifiedMonitoring runs unified status and health monitoring
func (mb *MQTTBridge) runUnifiedMonitoring() {
	statusTicker := time.NewTicker(time.Duration(mb.config.App.StatusIntervalSeconds) * time.Second)
	healthTicker := time.NewTicker(15 * time.Second) // 간격 조정 (10초 -> 15초)
	defer statusTicker.Stop()
	defer healthTicker.Stop()

	consecutiveFailures := 0
	const maxFailures = 3

	for {
		select {
		case <-statusTicker.C:
			// Combined status monitoring
			status := mb.mqttClient.GetConnectionStatus()
			reconnectCount := mb.mqttClient.GetReconnectCount()

			// Print unified status
			log.Printf("📊 === MQTT 브릿지 상태 ===")
			log.Printf("   MQTT: %s (재연결: %d회)", status, reconnectCount)

			// Print robot status summary
			mb.statusMonitor.PrintStatusSummary()

			// Check battery levels
			mb.statusMonitor.CheckBatteryLevels()

			log.Printf("   ========================")

		case <-healthTicker.C:
			// Health check only (no duplicate logging)
			if !mb.mqttClient.IsConnected() {
				consecutiveFailures++
				status := mb.mqttClient.GetConnectionStatus()

				if consecutiveFailures >= maxFailures {
					log.Printf("🚨 MQTT 연결 심각 - %d회 연속 실패 (상태: %s)", consecutiveFailures, status)
				}
			} else {
				if consecutiveFailures > 0 {
					log.Printf("✅ MQTT 연결 복구 (이전 실패: %d회)", consecutiveFailures)
				}
				consecutiveFailures = 0
			}

		case <-mb.statusMonitorStop:
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

	// Stop monitoring goroutine
	close(mb.statusMonitorStop)

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
