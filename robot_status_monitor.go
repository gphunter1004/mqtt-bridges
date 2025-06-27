package main

import (
	"fmt"
	"log"
	"strings"
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

// PrintStatusSummary prints a summary of all robot statuses
func (rsm *RobotStatusMonitor) PrintStatusSummary() {
	onlineRobots := rsm.robotManager.GetOnlineRobots()
	allRobots := rsm.robotManager.GetAllRobots()
	registeredTargetRobots := rsm.robotManager.GetRegisteredTargetRobots()
	missingTargetRobots := rsm.robotManager.GetMissingTargetRobots()
	targetRobotCount := rsm.robotManager.GetTargetRobotCount()

	log.Printf("   로봇 상태 - 대상: %d대, 등록: %d대, 온라인: %d대",
		targetRobotCount, len(registeredTargetRobots), len(onlineRobots))

	if len(missingTargetRobots) > 0 {
		log.Printf("   ⚠️  미등록 대상 로봇: %v", missingTargetRobots)
	}

	if len(registeredTargetRobots) > 0 {
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

			// Show data source info
			dataSourceInfo := ""
			if robot.HasConnectionInfo && robot.HasStateInfo {
				dataSourceInfo = "[연결+상태]"
			} else if robot.HasConnectionInfo {
				dataSourceInfo = "[연결만]"
			} else if robot.HasStateInfo {
				dataSourceInfo = "[상태만]"
			}

			log.Printf("   %s %s %s %s: %s",
				statusIcon, factsheetIcon, serialNumber, dataSourceInfo, robot.ConnectionState)

			// Show additional info if detailed status available
			if robot.HasDetailedInfo && robot.DetailedStatus != nil {
				details := []string{}

				if robot.BatteryLevel > 0 {
					batteryInfo := fmt.Sprintf("배터리 %.1f%%", robot.BatteryLevel)
					if robot.IsCharging {
						batteryInfo += "⚡"
					}
					details = append(details, batteryInfo)
				}

				if robot.IsExecutingOrder {
					orderInfo := fmt.Sprintf("주문: %s", robot.CurrentOrderID)
					if robot.IsDriving {
						orderInfo += " (주행중)"
					} else if robot.IsPaused {
						orderInfo += " (일시정지)"
					}
					details = append(details, orderInfo)
				}

				if len(robot.ActiveActions) > 0 {
					details = append(details, fmt.Sprintf("액션 %d개", len(robot.ActiveActions)))
				}

				if robot.HasErrors {
					details = append(details, "⚠️ 오류")
				}

				if robot.HasSafetyIssue {
					details = append(details, "🚨 안전")
				}

				if len(details) > 0 {
					log.Printf("     └─ %s", strings.Join(details, " | "))
				}
			}
		}
	}

	// Show non-target robots if any (for debugging)
	nonTargetCount := len(allRobots) - len(registeredTargetRobots)
	if nonTargetCount > 0 {
		log.Printf("   📋 대상 외 로봇: %d대", nonTargetCount)
	}
}

// CheckBatteryLevels checks for low battery warnings
func (rsm *RobotStatusMonitor) CheckBatteryLevels() {
	batteryStatuses := rsm.robotManager.GetRobotBatteryStatus()

	lowBatteryCount := 0
	for serial, battery := range batteryStatuses {
		// Use correct field names: BatteryCharge and Charging
		if battery.BatteryCharge < 20.0 && !battery.Charging {
			log.Printf("   🚨 배터리 부족: %s (%.1f%%)", serial, battery.BatteryCharge)
			lowBatteryCount++
		}
	}

	if lowBatteryCount == 0 && len(batteryStatuses) > 0 {
		log.Printf("   🔋 배터리 상태: 정상 (%d대)", len(batteryStatuses))
	}
}

// formatChargingStatus formats charging status with icon
func formatChargingStatus(isCharging bool) string {
	if isCharging {
		return "⚡"
	}
	return ""
}
