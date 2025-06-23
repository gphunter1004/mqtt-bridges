package main

import (
	"log"
	"time"
)

// RobotStatusMonitor handles robot status monitoring and automated responses
type RobotStatusMonitor struct {
	robotManager     *RobotManager
	messageProcessor *MessageProcessor
	config           *Config
}

// NewRobotStatusMonitor creates a new robot status monitor
func NewRobotStatusMonitor(robotManager *RobotManager, messageProcessor *MessageProcessor, config *Config) *RobotStatusMonitor {
	monitor := &RobotStatusMonitor{
		robotManager:     robotManager,
		messageProcessor: messageProcessor,
		config:           config,
	}

	// Set status change callback
	robotManager.SetStatusChangeCallback(monitor.handleRobotStatusChange)

	return monitor
}

// handleRobotStatusChange handles robot status changes and sends init command when robot comes online
func (rsm *RobotStatusMonitor) handleRobotStatusChange(serialNumber string, oldState, newState ConnectionState) {
	// Check if auto init is enabled
	if !rsm.config.App.AutoInitOnConnect {
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
			delayDuration := time.Duration(rsm.config.App.AutoInitDelaySec) * time.Second
			log.Printf("⏳ 자동 초기화 대기 중 (%ds): %s", rsm.config.App.AutoInitDelaySec, serialNumber)
			time.Sleep(delayDuration)

			// Check if robot is still online
			if !rsm.robotManager.IsRobotOnline(serialNumber) {
				log.Printf("⚠️  로봇 오프라인 됨 - 자동 초기화 취소: %s", serialNumber)
				return
			}

			// Send init action
			if err := rsm.sendActionToRobot(initAction, serialNumber); err != nil {
				log.Printf("❌ 자동 위치 초기화 실패 - Serial: %s, Error: %v", serialNumber, err)
				return
			}

			log.Printf("✅ 자동 위치 초기화 완료 - Serial: %s", serialNumber)

			// After successful init, request factsheet if enabled
			if rsm.config.App.AutoFactsheetRequest {
				if robot, exists := rsm.robotManager.GetRobotStatus(serialNumber); exists {
					// Wait a bit more for init to complete before requesting factsheet
					time.Sleep(1 * time.Second)

					log.Printf("📋 Factsheet 요청 시작 - Serial: %s", serialNumber)
					if err := rsm.messageProcessor.SendFactsheetRequest(serialNumber, robot.Manufacturer); err != nil {
						log.Printf("❌ Factsheet 요청 실패 - Serial: %s, Error: %v", serialNumber, err)
					} else {
						log.Printf("✅ Factsheet 요청 완료 - Serial: %s", serialNumber)
					}
				}
			}
		}()
	}
}

// sendActionToRobot is a helper method to send actions via message processor
func (rsm *RobotStatusMonitor) sendActionToRobot(plcAction *PLCActionMessage, serialNumber string) error {
	// Use the message processor to send the action
	return rsm.messageProcessor.sendActionToRobot(plcAction, serialNumber)
}

// PrintDetailedStatusReport prints a comprehensive status report for all robots
func (rsm *RobotStatusMonitor) PrintDetailedStatusReport() {
	robotsWithDetailedStatus := rsm.robotManager.GetRobotsWithDetailedStatus()

	if len(robotsWithDetailedStatus) == 0 {
		return
	}

	log.Printf("📊 === AGV 상세 상태 리포트 ===")

	for serial, robot := range robotsWithDetailedStatus {
		if robot.DetailedStatus == nil {
			continue
		}

		detailed := robot.DetailedStatus
		log.Printf("   🤖 %s (%s):", serial, robot.Manufacturer)

		// Position and movement
		pos := detailed.AGVPosition
		vel := detailed.Velocity
		log.Printf("     📍 위치: (%.3f, %.3f) 각도: %.2f° 점수: %.2f",
			pos.X, pos.Y, pos.Theta*180/3.14159, pos.LocalizationScore)
		log.Printf("     🚀 속도: vx=%.2f vy=%.2f ω=%.2f",
			vel.VX, vel.VY, vel.Omega)

		// Battery details
		battery := detailed.BatteryState
		log.Printf("     🔋 배터리: %.1f%% (%.1fV) 건강도: %d 충전: %t",
			battery.BatteryCharge, battery.BatteryVoltage,
			battery.BatteryHealth, battery.Charging)

		// Safety status
		safety := detailed.SafetyState
		log.Printf("     🛡️  안전: E-Stop=%s 영역위반=%t",
			safety.EStop, safety.FieldViolation)

		// Current tasks
		log.Printf("     📋 작업: 주행=%t 일시정지=%t 모드=%s",
			detailed.Driving, detailed.Paused, detailed.OperatingMode)

		if detailed.OrderID != "" {
			log.Printf("     📦 주문: %s (업데이트: %d) 마지막노드: %s",
				detailed.OrderID, detailed.OrderUpdateID, detailed.LastNodeID)
		}

		// Actions
		if len(detailed.ActionStates) > 0 {
			log.Printf("     🎯 액션 (%d개):", len(detailed.ActionStates))
			for i, action := range detailed.ActionStates {
				log.Printf("       %d. %s: %s", i+1, action.ActionType, action.ActionStatus)
			}
		}

		// Nodes and edges
		if len(detailed.NodeStates) > 0 {
			log.Printf("     🗺️  노드 (%d개): ", len(detailed.NodeStates))
			for _, node := range detailed.NodeStates {
				log.Printf("       %s seq=%d (%.2f,%.2f,%.2f°)",
					node.NodeID, node.SequenceID,
					node.NodePosition.X, node.NodePosition.Y,
					node.NodePosition.Theta*180/3.14159)
			}
		}

		if len(detailed.EdgeStates) > 0 {
			log.Printf("     🔗 엣지 (%d개):", len(detailed.EdgeStates))
			for _, edge := range detailed.EdgeStates {
				log.Printf("       %s seq=%d -> %s",
					edge.EdgeID, edge.SequenceID, edge.EndNodeID)
			}
		}

		// Errors and warnings
		if len(detailed.Errors) > 0 {
			log.Printf("     ❌ 에러 (%d개): %v", len(detailed.Errors), detailed.Errors)
		}
		if len(detailed.Information) > 0 {
			log.Printf("     ℹ️  정보 (%d개): %v", len(detailed.Information), detailed.Information)
		}

		log.Printf("     ⏰ 마지막 업데이트: %s",
			robot.DetailedUpdate.Format("2006-01-02 15:04:05"))
		log.Printf("     ---")
	}
	log.Printf("   ===============================")
}

// PrintStatusSummary prints a summary of all robot statuses
func (rsm *RobotStatusMonitor) PrintStatusSummary() {
	onlineRobots := rsm.robotManager.GetOnlineRobots()
	allRobots := rsm.robotManager.GetAllRobots()
	registeredTargetRobots := rsm.robotManager.GetRegisteredTargetRobots()
	missingTargetRobots := rsm.robotManager.GetMissingTargetRobots()
	targetRobotCount := rsm.robotManager.GetTargetRobotCount()
	robotsWithDetailedStatus := rsm.robotManager.GetRobotsWithDetailedStatus()

	log.Printf("📊 === 시스템 상태 요약 ===")
	log.Printf("   대상 로봇: %d대 (등록: %d대, 미등록: %d대)",
		targetRobotCount, len(registeredTargetRobots), len(missingTargetRobots))
	log.Printf("   로봇 현황 - 총: %d대, 온라인: %d대, 상세정보: %d대",
		len(allRobots), len(onlineRobots), len(robotsWithDetailedStatus))

	if len(missingTargetRobots) > 0 {
		log.Printf("   ⚠️  미등록 대상 로봇: %v", missingTargetRobots)
	}

	if len(registeredTargetRobots) > 0 {
		log.Printf("   등록된 대상 로봇:")
		for serialNumber, robot := range registeredTargetRobots {
			statusIcon := "🔴"
			if robot.IsOnline {
				statusIcon = "🟢"
			} else if robot.ConnectionState == ConnectionBroken {
				statusIcon = "🟡"
			}

			factsheetIcon := ""
			if robot.HasFactsheet {
				factsheetIcon = "📋"
			}

			detailedIcon := ""
			if robot.HasDetailedInfo {
				detailedIcon = "📊"
			}

			// Display basic info
			log.Printf("     %s %s %s %s: %s (업데이트: %s)",
				statusIcon, factsheetIcon, detailedIcon, serialNumber, robot.ConnectionState,
				robot.LastUpdate.Format("15:04:05"))

			// Display detailed status if available
			if robot.HasDetailedInfo && robot.DetailedStatus != nil {
				log.Printf("       🔋 배터리: %.1f%% %s| 🚗 주행: %t | ⏸️  일시정지: %t | ⚙️  모드: %s",
					robot.BatteryLevel,
					formatChargingStatus(robot.IsCharging),
					robot.IsDriving,
					robot.IsPaused,
					robot.OperatingMode)

				if robot.CurrentOrderID != "" {
					log.Printf("       📦 주문: %s | 🎯 액션: %d개 | 📍 노드: %s",
						robot.CurrentOrderID,
						robot.ActiveActions,
						robot.DetailedStatus.LastNodeID)
				}

				if robot.HasErrors {
					log.Printf("       ⚠️  에러: %d개", len(robot.DetailedStatus.Errors))
				}

				// Position info
				pos := robot.DetailedStatus.AGVPosition
				log.Printf("       📍 위치: (%.2f, %.2f, %.1f°) | 🎯 정확도: %.2f",
					pos.X, pos.Y, pos.Theta*180/3.14159, pos.LocalizationScore)
			}
		}
	}

	// Show non-target robots if any (for debugging)
	nonTargetRobots := make(map[string]*RobotStatus)
	for serialNumber, robot := range allRobots {
		if !rsm.robotManager.IsTargetRobot(serialNumber) {
			nonTargetRobots[serialNumber] = robot
		}
	}

	if len(nonTargetRobots) > 0 {
		log.Printf("   📋 대상 외 로봇 (%d대):", len(nonTargetRobots))
		for serialNumber, robot := range nonTargetRobots {
			log.Printf("     ℹ️  %s: %s", serialNumber, robot.ConnectionState)
		}
	}

	rsm.printBatterySummary()
	rsm.printActiveOrdersSummary()

	log.Printf("   ========================")
}

// printBatterySummary prints battery status summary
func (rsm *RobotStatusMonitor) printBatterySummary() {
	batteryStatuses := rsm.robotManager.GetRobotBatteryStatus()
	if len(batteryStatuses) == 0 {
		return
	}

	log.Printf("   🔋 배터리 상태 요약:")
	for serial, battery := range batteryStatuses {
		chargingStatus := ""
		if battery.IsCharging {
			chargingStatus = " (충전중)"
		}
		log.Printf("     %s: %.1f%%%s (전압: %.1fV)",
			serial, battery.BatteryLevel, chargingStatus, battery.BatteryVoltage)
	}
}

// printActiveOrdersSummary prints active orders summary
func (rsm *RobotStatusMonitor) printActiveOrdersSummary() {
	activeOrders := rsm.robotManager.GetActiveRobotOrders()
	if len(activeOrders) == 0 {
		return
	}

	log.Printf("   📦 활성 주문 요약:")
	for serial, order := range activeOrders {
		statusText := ""
		if order.IsPaused {
			statusText = " (일시정지)"
		} else if order.IsDriving {
			statusText = " (주행중)"
		}
		log.Printf("     %s: %s - 액션 %d개%s",
			serial, order.OrderID, order.ActiveActions, statusText)
	}
}

// CheckBatteryLevels checks for low battery warnings
func (rsm *RobotStatusMonitor) CheckBatteryLevels() {
	batteryStatuses := rsm.robotManager.GetRobotBatteryStatus()

	for serial, battery := range batteryStatuses {
		if battery.BatteryLevel < 20.0 && !battery.IsCharging {
			log.Printf("🚨 배터리 부족 경고 - %s: %.1f%%", serial, battery.BatteryLevel)
		}
	}
}

// CheckRobotErrors checks for robots with errors and logs warnings
func (rsm *RobotStatusMonitor) CheckRobotErrors() {
	robotsWithDetailedStatus := rsm.robotManager.GetRobotsWithDetailedStatus()

	for serial, robot := range robotsWithDetailedStatus {
		if robot.HasErrors && robot.DetailedStatus != nil {
			log.Printf("⚠️  로봇 에러 감지 - %s: %d개 에러", serial, len(robot.DetailedStatus.Errors))
			for i, err := range robot.DetailedStatus.Errors {
				log.Printf("     %d. %v", i+1, err)
			}
		}
	}
}

// CheckSafetyIssues checks for safety-related issues
func (rsm *RobotStatusMonitor) CheckSafetyIssues() {
	robotsWithDetailedStatus := rsm.robotManager.GetRobotsWithDetailedStatus()

	for serial, robot := range robotsWithDetailedStatus {
		if robot.DetailedStatus == nil {
			continue
		}

		safety := robot.DetailedStatus.SafetyState

		// Check E-Stop status
		if safety.EStop != "NONE" {
			log.Printf("🚨 E-Stop 활성화 - %s: %s", serial, safety.EStop)
		}

		// Check field violations
		if safety.FieldViolation {
			log.Printf("🚨 필드 위반 감지 - %s", serial)
		}
	}
}

// GetRobotHealthSummary returns a summary of robot health status
func (rsm *RobotStatusMonitor) GetRobotHealthSummary() RobotHealthSummary {
	onlineRobots := rsm.robotManager.GetOnlineRobots()
	batteryStatuses := rsm.robotManager.GetRobotBatteryStatus()
	robotsWithDetailedStatus := rsm.robotManager.GetRobotsWithDetailedStatus()

	summary := RobotHealthSummary{
		TotalOnlineRobots:    len(onlineRobots),
		RobotsWithLowBattery: 0,
		RobotsWithErrors:     0,
		RobotsCharging:       0,
		RobotsDriving:        0,
		AverageBatteryLevel:  0.0,
		LastCheckTime:        time.Now(),
	}

	// Calculate battery statistics
	totalBatteryLevel := 0.0
	for _, battery := range batteryStatuses {
		totalBatteryLevel += battery.BatteryLevel

		if battery.BatteryLevel < 20.0 {
			summary.RobotsWithLowBattery++
		}

		if battery.IsCharging {
			summary.RobotsCharging++
		}
	}

	if len(batteryStatuses) > 0 {
		summary.AverageBatteryLevel = totalBatteryLevel / float64(len(batteryStatuses))
	}

	// Calculate error and driving statistics
	for _, robot := range robotsWithDetailedStatus {
		if robot.HasErrors {
			summary.RobotsWithErrors++
		}

		if robot.IsDriving {
			summary.RobotsDriving++
		}
	}

	return summary
}

// formatChargingStatus formats charging status with icon
func formatChargingStatus(isCharging bool) string {
	if isCharging {
		return "⚡"
	}
	return ""
}

// RobotHealthSummary represents overall robot health statistics
type RobotHealthSummary struct {
	TotalOnlineRobots    int       `json:"totalOnlineRobots"`
	RobotsWithLowBattery int       `json:"robotsWithLowBattery"`
	RobotsWithErrors     int       `json:"robotsWithErrors"`
	RobotsCharging       int       `json:"robotsCharging"`
	RobotsDriving        int       `json:"robotsDriving"`
	AverageBatteryLevel  float64   `json:"averageBatteryLevel"`
	LastCheckTime        time.Time `json:"lastCheckTime"`
}
