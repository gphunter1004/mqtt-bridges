package main

import (
	"log"
	"sync"
	"time"
)

// StatusChangeCallback is a function type for handling robot status changes
type StatusChangeCallback func(serialNumber string, oldState, newState ConnectionState)

// RobotManager manages multiple robots' connection states
type RobotManager struct {
	robots               map[string]*RobotStatus
	targetSerials        map[string]bool // 관리 대상 로봇 시리얼 번호 목록
	mutex                sync.RWMutex
	statusChangeCallback StatusChangeCallback // 상태 변경 콜백
}

// NewRobotManager creates a new robot manager with target serials
func NewRobotManager(targetSerials []string) *RobotManager {
	// Create target serials map for quick lookup
	targetMap := make(map[string]bool)
	for _, serial := range targetSerials {
		targetMap[serial] = true
	}

	return &RobotManager{
		robots:        make(map[string]*RobotStatus),
		targetSerials: targetMap,
	}
}

// SetStatusChangeCallback sets the callback function for status changes
func (rm *RobotManager) SetStatusChangeCallback(callback StatusChangeCallback) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()
	rm.statusChangeCallback = callback
}

// UpdateRobotConnectionStatus updates robot status from basic connection message
func (rm *RobotManager) UpdateRobotConnectionStatus(msg *RobotConnectionMessage) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	serialNumber := msg.SerialNumber

	// Check if this robot is in target list
	if !rm.targetSerials[serialNumber] {
		log.Printf("⚠️  관리 대상이 아닌 로봇 연결 메시지 무시 - Serial: %s", serialNumber)
		return
	}

	// Get existing robot or create new one
	robot, exists := rm.robots[serialNumber]
	if !exists {
		robot = &RobotStatus{
			SerialNumber: serialNumber,
			Manufacturer: msg.Manufacturer,
		}
		rm.robots[serialNumber] = robot
		log.Printf("✅ 새로운 로봇 등록 (연결) - Serial: %s", serialNumber)
	}

	// Check if this is a newer message
	if robot.HasConnectionInfo && robot.LastHeaderID > msg.HeaderID {
		log.Printf("⚠️  이전 연결 메시지 무시 - Robot: %s, Current HeaderID: %d, Received HeaderID: %d",
			serialNumber, robot.LastHeaderID, msg.HeaderID)
		return
	}

	// Store previous state for comparison
	previousState := robot.ConnectionState

	// Update connection info
	robot.ConnectionState = msg.ConnectionState
	robot.LastUpdate = time.Now()
	robot.ConnectionUpdate = time.Now()
	robot.LastHeaderID = msg.HeaderID
	robot.IsOnline = (msg.ConnectionState == Online)
	robot.HasConnectionInfo = true

	// Log state changes
	if previousState != msg.ConnectionState {
		log.Printf("🔄 로봇 연결 상태 변경 - Serial: %s, %s -> %s",
			serialNumber, previousState, msg.ConnectionState)

		// Call status change callback if set
		if rm.statusChangeCallback != nil {
			// Release lock before calling callback to avoid deadlock
			rm.mutex.Unlock()
			rm.statusChangeCallback(serialNumber, previousState, msg.ConnectionState)
			rm.mutex.Lock()
		}
	}
}

// UpdateRobotStateStatus updates detailed robot status from state messages
func (rm *RobotManager) UpdateRobotStateStatus(stateMsg *RobotStateMessage) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	serialNumber := stateMsg.SerialNumber

	// Check if this robot is in target list
	if !rm.targetSerials[serialNumber] {
		log.Printf("⚠️  관리 대상이 아닌 로봇 상태 메시지 무시 - Serial: %s", serialNumber)
		return
	}

	// Get existing robot or create new one
	robot, exists := rm.robots[serialNumber]
	if !exists {
		robot = &RobotStatus{
			SerialNumber: serialNumber,
			Manufacturer: stateMsg.Manufacturer,
		}
		rm.robots[serialNumber] = robot
		log.Printf("✅ 새로운 로봇 등록 (상태) - Serial: %s", serialNumber)
	}

	// Update detailed status
	robot.DetailedStatus = stateMsg
	robot.DetailedUpdate = time.Now()
	robot.StateUpdate = time.Now()
	robot.HasDetailedInfo = true
	robot.HasStateInfo = true

	// Update basic info from state message if we don't have connection info or this is newer
	if !robot.HasConnectionInfo || stateMsg.HeaderID > robot.LastHeaderID {
		robot.LastUpdate = time.Now()
		robot.LastHeaderID = stateMsg.HeaderID
		robot.Manufacturer = stateMsg.Manufacturer
	}

	// Update order execution state from detailed status
	robot.CurrentOrderID = stateMsg.OrderID
	robot.OrderUpdateID = stateMsg.OrderUpdateID
	robot.IsExecutingOrder = (stateMsg.OrderID != "")
	robot.IsDriving = stateMsg.Driving
	robot.IsPaused = stateMsg.Paused
	robot.OperatingMode = stateMsg.OperatingMode
	robot.LastNodeID = stateMsg.LastNodeID

	// Update position and battery from detailed status
	robot.CurrentPosition = &stateMsg.AGVPosition
	robot.BatteryLevel = stateMsg.BatteryState.BatteryCharge // 실제 필드명 사용
	robot.IsCharging = stateMsg.BatteryState.Charging        // 실제 필드명 사용

	// Update error status
	robot.HasErrors = len(stateMsg.Errors) > 0
	if len(stateMsg.Errors) > 0 {
		robot.LastError = &stateMsg.Errors[0]
	} else {
		robot.LastError = nil
	}

	// Update safety status
	robot.HasSafetyIssue = (stateMsg.SafetyState.EStop != "NONE" || stateMsg.SafetyState.FieldViolation)

	// Update active actions
	robot.ActiveActions = make([]ActionState, len(stateMsg.ActionStates))
	copy(robot.ActiveActions, stateMsg.ActionStates)

	// Set order start time if this is a new order
	if robot.IsExecutingOrder && robot.OrderStartTime.IsZero() {
		robot.OrderStartTime = time.Now()
	} else if !robot.IsExecutingOrder {
		robot.OrderStartTime = time.Time{} // Reset if no order
	}
}

// GetRobotStatus returns the current status of a robot
func (rm *RobotManager) GetRobotStatus(serialNumber string) (*RobotStatus, bool) {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	robot, exists := rm.robots[serialNumber]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent race conditions
	robotCopy := *robot
	return &robotCopy, true
}

// GetAllRobots returns all robots' status
func (rm *RobotManager) GetAllRobots() map[string]*RobotStatus {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RobotStatus)
	for k, v := range rm.robots {
		robotCopy := *v
		result[k] = &robotCopy
	}
	return result
}

// IsRobotOnline checks if a robot is online
func (rm *RobotManager) IsRobotOnline(serialNumber string) bool {
	robot, exists := rm.GetRobotStatus(serialNumber)
	return exists && robot.ConnectionState == Online
}

// GetOnlineRobots returns all online robots
func (rm *RobotManager) GetOnlineRobots() []string {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	var onlineRobots []string
	for serialNumber, robot := range rm.robots {
		if robot.ConnectionState == Online {
			onlineRobots = append(onlineRobots, serialNumber)
		}
	}
	return onlineRobots
}

// GetTargetSerials returns the list of target robot serials
func (rm *RobotManager) GetTargetSerials() []string {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	var serials []string
	for serial := range rm.targetSerials {
		serials = append(serials, serial)
	}
	return serials
}

// IsTargetRobot checks if a robot serial is in the target list
func (rm *RobotManager) IsTargetRobot(serialNumber string) bool {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	return rm.targetSerials[serialNumber]
}

// GetTargetRobotCount returns the total number of target robots
func (rm *RobotManager) GetTargetRobotCount() int {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()
	return len(rm.targetSerials)
}

// GetRegisteredTargetRobots returns target robots that have been registered (sent status at least once)
func (rm *RobotManager) GetRegisteredTargetRobots() map[string]*RobotStatus {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RobotStatus)
	for k, v := range rm.robots {
		if rm.targetSerials[k] {
			robotCopy := *v
			result[k] = &robotCopy
		}
	}
	return result
}

// GetMissingTargetRobots returns target robots that haven't registered yet
func (rm *RobotManager) GetMissingTargetRobots() []string {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	var missing []string
	for serial := range rm.targetSerials {
		if _, exists := rm.robots[serial]; !exists {
			missing = append(missing, serial)
		}
	}
	return missing
}

// UpdateFactsheetReceived updates the factsheet received status for a robot
func (rm *RobotManager) UpdateFactsheetReceived(serialNumber string) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if robot, exists := rm.robots[serialNumber]; exists {
		robot.HasFactsheet = true
		robot.FactsheetUpdate = time.Now()
		log.Printf("📋 로봇 Factsheet 수신 완료 - Serial: %s", serialNumber)
	}
}

// GetExecutingRobots returns robots currently executing orders
func (rm *RobotManager) GetExecutingRobots() map[string]*RobotStatus {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RobotStatus)
	for k, v := range rm.robots {
		if v.IsExecutingOrder && rm.targetSerials[k] {
			robotCopy := *v
			result[k] = &robotCopy
		}
	}
	return result
}

// GetRobotsWithErrors returns robots that have errors
func (rm *RobotManager) GetRobotsWithErrors() map[string]*RobotStatus {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RobotStatus)
	for k, v := range rm.robots {
		if (v.LastError != nil || v.HasSafetyIssue) && rm.targetSerials[k] {
			robotCopy := *v
			result[k] = &robotCopy
		}
	}
	return result
}

// GetLowBatteryRobots returns robots with low battery
func (rm *RobotManager) GetLowBatteryRobots(threshold float64) map[string]*RobotStatus {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RobotStatus)
	for k, v := range rm.robots {
		if v.BatteryLevel > 0 && v.BatteryLevel < threshold && rm.targetSerials[k] {
			robotCopy := *v
			result[k] = &robotCopy
		}
	}
	return result
}

// GetRobotsWithDetailedStatus returns robots that have detailed status information
func (rm *RobotManager) GetRobotsWithDetailedStatus() map[string]*RobotStatus {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]*RobotStatus)
	for k, v := range rm.robots {
		if v.HasDetailedInfo && rm.targetSerials[k] {
			robotCopy := *v
			result[k] = &robotCopy
		}
	}
	return result
}

// GetRobotBatteryStatus returns battery status for all robots with detailed info
func (rm *RobotManager) GetRobotBatteryStatus() map[string]BatteryState {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]BatteryState)
	for k, v := range rm.robots {
		if v.HasDetailedInfo && v.DetailedStatus != nil && rm.targetSerials[k] {
			result[k] = v.DetailedStatus.BatteryState // 직접 BatteryState 반환
		}
	}
	return result
}

// GetActiveRobotOrders returns robots currently executing orders
func (rm *RobotManager) GetActiveRobotOrders() map[string]ActiveOrder {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	result := make(map[string]ActiveOrder)
	for k, v := range rm.robots {
		if v.IsExecutingOrder && rm.targetSerials[k] {
			result[k] = ActiveOrder{
				OrderID:       v.CurrentOrderID,
				IsDriving:     v.IsDriving,
				IsPaused:      v.IsPaused,
				ActiveActions: len(v.ActiveActions),
				StartTime:     v.OrderStartTime,
			}
		}
	}
	return result
}
