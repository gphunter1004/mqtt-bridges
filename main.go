package main

import (
	"log"
	"os"
	"os/signal"
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
	log.Printf("      - Robot Orders: meili/v2/Roboligent/{serial}/orders")
	log.Printf("   💡 종료하려면 Ctrl+C를 누르세요")

	// Wait for shutdown signal (모든 모니터링은 bridge 내부에서 처리)
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
