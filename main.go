package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"
)

func main() {
	log.Printf("🚀 MQTT Robot Bridge 시작...")

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("❌ 설정 로드 실패: %v", err)
	}

	log.Printf("📋 설정 로드 완료")
	log.Printf("   - Environment: %s", config.App.Environment)
	log.Printf("   - MQTT Broker: %s", config.MQTT.BrokerURL)
	log.Printf("   - MQTT Client ID: %s", config.MQTT.ClientID)
	log.Printf("   - Target Robots: %v", config.App.TargetRobotSerials)
	log.Printf("   - Auto Init on Connect: %t (Delay: %ds)", config.App.AutoInitOnConnect, config.App.AutoInitDelaySec)
	log.Printf("   - Log Level: %s", config.App.LogLevel)
	log.Printf("   - Status Interval: %ds", config.App.StatusIntervalSeconds)
	log.Printf("   - Max Reconnect Attempts: %d", config.MQTT.MaxReconnectAttempts)

	// Create and start MQTT bridge
	bridge := NewMQTTBridge(config)

	// Setup graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Start bridge in goroutine
	bridgeError := make(chan error, 1)
	go func() {
		if err := bridge.Start(); err != nil {
			bridgeError <- err
		}
	}()

	// Check for immediate startup errors
	select {
	case err := <-bridgeError:
		log.Fatalf("❌ 브릿지 시작 실패: %v", err)
	case <-time.After(2 * time.Second):
		// Bridge started successfully
	}

	// Check connection status after startup
	if !bridge.IsConnected() {
		status := bridge.GetConnectionStatus()
		log.Printf("⚠️  MQTT 연결 실패 - 상태: %s", status)
		log.Printf("   재연결을 시도하고 있습니다...")
	} else {
		log.Printf("✅ MQTT 연결 완료")
	}

	log.Printf("🎯 MQTT 브릿지가 작동 중입니다...")
	log.Printf("   📥 구독 토픽:")
	log.Printf("      - PLC Actions: bridge/actions")
	log.Printf("      - Robot Status: meili/v2/Roboligent/+/connection")
	log.Printf("      - Robot Factsheet: meili/v2/+/+/factsheet")
	log.Printf("   📤 발행 토픽:")
	log.Printf("      - Robot Actions: meili/v2/Roboligent/{serial}/instantActions")
	log.Printf("   💡 종료하려면 Ctrl+C를 누르세요")

	// Status monitoring goroutine - 확장된 상태 정보 표시
	go func() {
		ticker := time.NewTicker(time.Duration(config.App.StatusIntervalSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				robotManager := bridge.GetRobotManager()
				onlineRobots := robotManager.GetOnlineRobots()
				allRobots := robotManager.GetAllRobots()
				registeredTargetRobots := robotManager.GetRegisteredTargetRobots()
				missingTargetRobots := robotManager.GetMissingTargetRobots()
				targetRobotCount := robotManager.GetTargetRobotCount()
				executingRobots := robotManager.GetExecutingRobots()
				robotsWithErrors := robotManager.GetRobotsWithErrors()
				lowBatteryRobots := robotManager.GetLowBatteryRobots(25.0)

				// Connection status
				status := bridge.GetConnectionStatus()

				log.Printf("📊 === 시스템 상태 요약 ===")
				log.Printf("   MQTT 연결: %s", status)
				log.Printf("   대상 로봇: %d대 (등록: %d대, 미등록: %d대)",
					targetRobotCount, len(registeredTargetRobots), len(missingTargetRobots))
				log.Printf("   로봇 현황 - 총: %d대, 온라인: %d대, 실행중: %d대",
					len(allRobots), len(onlineRobots), len(executingRobots))

				// Alert summary
				alertCount := 0
				if len(robotsWithErrors) > 0 {
					alertCount += len(robotsWithErrors)
				}
				if len(lowBatteryRobots) > 0 {
					alertCount += len(lowBatteryRobots)
				}
				if alertCount > 0 {
					log.Printf("   🚨 알림: %d개 (에러: %d대, 배터리부족: %d대)",
						alertCount, len(robotsWithErrors), len(lowBatteryRobots))
				}

				if len(missingTargetRobots) > 0 {
					log.Printf("   ⚠️  미등록 대상 로봇: %v", missingTargetRobots)
				}

				if len(registeredTargetRobots) > 0 {
					log.Printf("   등록된 대상 로봇:")
					for serialNumber, robot := range registeredTargetRobots {
						statusIcon := "🔴"
						if robot.ConnectionState == Online {
							statusIcon = "🟢"
						} else if robot.ConnectionState == ConnectionBroken {
							statusIcon = "🟡"
						}

						factsheetIcon := ""
						if robot.HasFactsheet {
							factsheetIcon = "📋"
						}

						// 추가 상태 아이콘들
						orderIcon := ""
						if robot.IsExecutingOrder {
							if robot.IsDriving {
								orderIcon = "🚛"
							} else if robot.IsPaused {
								orderIcon = "⏸️"
							} else {
								orderIcon = "📋"
							}
						}

						batteryIcon := ""
						if robot.BatteryLevel > 0 {
							if robot.BatteryLevel < 20 {
								batteryIcon = "🪫"
							} else if robot.IsCharging {
								batteryIcon = "🔌"
							} else if robot.BatteryLevel < 50 {
								batteryIcon = "🔋"
							}
						}

						errorIcon := ""
						if robot.LastError != nil || robot.HasSafetyIssue {
							errorIcon = "🚨"
						}

						// 기본 정보
						log.Printf("     %s %s%s%s%s %s: %s (업데이트: %s)",
							statusIcon, factsheetIcon, orderIcon, batteryIcon, errorIcon,
							serialNumber, robot.ConnectionState,
							robot.LastUpdate.Format("15:04:05"))

						// 상세 상태 정보 (온라인인 경우만)
						if robot.ConnectionState == Online {
							details := []string{}

							if robot.IsExecutingOrder {
								orderInfo := "Order: " + robot.CurrentOrderID[:8]
								if robot.IsDriving {
									orderInfo += " (이동중)"
								} else if robot.IsPaused {
									orderInfo += " (일시정지)"
								}
								details = append(details, orderInfo)
							}

							if robot.BatteryLevel > 0 {
								batteryInfo := fmt.Sprintf("배터리: %.1f%%", robot.BatteryLevel)
								if robot.IsCharging {
									batteryInfo += " (충전중)"
								}
								details = append(details, batteryInfo)
							}

							if robot.CurrentPosition != nil {
								posInfo := fmt.Sprintf("위치: (%.1f, %.1f)",
									robot.CurrentPosition.X, robot.CurrentPosition.Y)
								details = append(details, posInfo)
							}

							if len(robot.ActiveActions) > 0 {
								actionInfo := fmt.Sprintf("액션: %d개", len(robot.ActiveActions))
								details = append(details, actionInfo)
							}

							if robot.LastError != nil {
								errorInfo := fmt.Sprintf("에러: %s", robot.LastError.ErrorType)
								details = append(details, errorInfo)
							}

							if len(details) > 0 {
								log.Printf("       └─ %s", strings.Join(details, " | "))
							}
						}
					}
				}

				// Show non-target robots if any (for debugging)
				nonTargetRobots := make(map[string]*RobotStatus)
				for serialNumber, robot := range allRobots {
					if !robotManager.IsTargetRobot(serialNumber) {
						nonTargetRobots[serialNumber] = robot
					}
				}

				if len(nonTargetRobots) > 0 {
					log.Printf("   📋 대상 외 로봇 (%d대):", len(nonTargetRobots))
					for serialNumber, robot := range nonTargetRobots {
						log.Printf("     ℹ️  %s: %s", serialNumber, robot.ConnectionState)
					}
				}

				// 상세 알림 표시
				if len(robotsWithErrors) > 0 {
					log.Printf("   🚨 에러 상태 로봇:")
					for serialNumber, robot := range robotsWithErrors {
						if robot.LastError != nil {
							log.Printf("     - %s: %s (%s)",
								serialNumber, robot.LastError.ErrorType, robot.LastError.ErrorDescription)
						}
						if robot.HasSafetyIssue {
							log.Printf("     - %s: 안전 문제 발생", serialNumber)
						}
					}
				}

				if len(lowBatteryRobots) > 0 {
					log.Printf("   🪫 배터리 부족 로봇:")
					for serialNumber, robot := range lowBatteryRobots {
						log.Printf("     - %s: %.1f%%", serialNumber, robot.BatteryLevel)
					}
				}

				log.Printf("   ========================")

				// Alert if MQTT connection is down
				if status != Connected {
					log.Printf("⚠️  MQTT 연결 문제 - 상태: %s", status)
				}

			case <-signalChan:
				return
			}
		}
	}()

	// Connection health check goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		consecutiveFailures := 0
		const maxFailures = 3

		for {
			select {
			case <-ticker.C:
				if !bridge.IsConnected() {
					consecutiveFailures++
					status := bridge.GetConnectionStatus()

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

			case <-signalChan:
				return
			}
		}
	}()

	// Wait for shutdown signal
	sig := <-signalChan
	log.Printf("🛑 종료 신호 수신: %v", sig)
	log.Printf("⏳ 안전한 종료를 위해 %d초 대기...", config.App.GracefulShutdownSec)

	// Graceful shutdown with timeout
	shutdownTimeout := time.Duration(config.App.GracefulShutdownSec) * time.Second
	shutdownComplete := make(chan struct{})

	go func() {
		bridge.Stop()
		close(shutdownComplete)
	}()

	select {
	case <-shutdownComplete:
		log.Printf("✅ 정상 종료 완료")
	case <-time.After(shutdownTimeout):
		log.Printf("⚠️  종료 타임아웃 - 강제 종료")
	}

	log.Printf("👋 MQTT Robot Bridge 종료됨")
}
