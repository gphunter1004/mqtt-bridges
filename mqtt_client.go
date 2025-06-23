package main

import (
	"context"
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

// MQTTClient handles MQTT connection and basic operations
type MQTTClient struct {
	client   mqtt.Client
	config   *MQTTConfig
	handlers *MessageHandlers

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

// MessageHandlers contains all message handling functions
type MessageHandlers struct {
	PLCActionHandler       mqtt.MessageHandler
	RobotConnectionHandler mqtt.MessageHandler
	RobotFactsheetHandler  mqtt.MessageHandler
}

// NewMQTTClient creates a new MQTT client
func NewMQTTClient(config *MQTTConfig, handlers *MessageHandlers) *MQTTClient {
	ctx, cancel := context.WithCancel(context.Background())

	client := &MQTTClient{
		config:                config,
		handlers:              handlers,
		status:                Disconnected,
		shutdownCtx:           ctx,
		shutdownCancel:        cancel,
		connectionMonitorStop: make(chan struct{}),
	}

	// Create MQTT client
	client.client = client.createMQTTClient()

	return client
}

// createMQTTClient creates the underlying MQTT client
func (mc *MQTTClient) createMQTTClient() mqtt.Client {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mc.config.BrokerURL)
	opts.SetClientID(mc.config.ClientID)

	if mc.config.Username != "" {
		opts.SetUsername(mc.config.Username)
		opts.SetPassword(mc.config.Password)
	}

	opts.SetKeepAlive(time.Duration(mc.config.KeepAlive) * time.Second)
	opts.SetConnectTimeout(time.Duration(mc.config.ConnectTimeout) * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetCleanSession(mc.config.CleanSession)
	opts.SetMaxReconnectInterval(time.Duration(mc.config.MaxReconnectDelay) * time.Second)

	// Connection handlers
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		mc.updateStatus(Connected)
		reconnectCount := atomic.LoadInt32(&mc.reconnectCount)
		if reconnectCount > 0 {
			log.Printf("✅ MQTT 재연결 성공 - Broker: %s (재연결 횟수: %d)", mc.config.BrokerURL, reconnectCount)
		} else {
			log.Printf("✅ MQTT 클라이언트 연결됨 - Broker: %s, ClientID: %s", mc.config.BrokerURL, mc.config.ClientID)
		}

		// Subscribe to all topics on (re)connection
		mc.subscribeToTopics()
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		mc.updateStatus(ConnectionLost)
		log.Printf("❌ MQTT 연결 끊어짐 - Error: %v", err)
		atomic.AddInt32(&mc.reconnectCount, 1)
	})

	opts.SetReconnectingHandler(func(client mqtt.Client, opts *mqtt.ClientOptions) {
		mc.updateStatus(Connecting)
		reconnectCount := atomic.LoadInt32(&mc.reconnectCount)
		log.Printf("🔄 MQTT 재연결 시도 중... (시도 횟수: %d)", reconnectCount)
	})

	return mqtt.NewClient(opts)
}

// updateStatus updates connection status
func (mc *MQTTClient) updateStatus(status ConnectionStatus) {
	mc.statusMutex.Lock()
	defer mc.statusMutex.Unlock()
	mc.status = status
}

// GetConnectionStatus returns current connection status
func (mc *MQTTClient) GetConnectionStatus() ConnectionStatus {
	mc.statusMutex.RLock()
	defer mc.statusMutex.RUnlock()
	return mc.status
}

// IsConnected checks if the client is connected
func (mc *MQTTClient) IsConnected() bool {
	return mc.GetConnectionStatus() == Connected && mc.client.IsConnected()
}

// Connect attempts to connect with retry logic
func (mc *MQTTClient) Connect() error {
	maxAttempts := mc.config.MaxReconnectAttempts
	connectTimeout := time.Duration(mc.config.ConnectTimeout) * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		log.Printf("🔌 MQTT 연결 시도 중... (%d/%d) - Broker: %s",
			attempt, maxAttempts, mc.config.BrokerURL)

		// Attempt connection
		token := mc.client.Connect()

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
			retryDelay := time.Duration(mc.config.ReconnectDelay) * time.Second
			log.Printf("⏳ %d초 후 재시도...", mc.config.ReconnectDelay)

			select {
			case <-time.After(retryDelay):
				// Continue to next attempt
			case <-mc.shutdownCtx.Done():
				return fmt.Errorf("연결 시도 중 종료 요청됨")
			}
		}
	}

	return fmt.Errorf("MQTT 연결 실패 - 최대 재시도 횟수 초과 (%d번)", maxAttempts)
}

// subscribeToTopics subscribes to all required topics
func (mc *MQTTClient) subscribeToTopics() {
	if !mc.client.IsConnected() {
		log.Printf("❌ MQTT 클라이언트가 연결되지 않아 토픽 구독 불가")
		return
	}

	// Subscribe to PLC action messages
	actionTopic := "bridge/actions"
	token := mc.client.Subscribe(actionTopic, mc.config.QoS, mc.handlers.PLCActionHandler)
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		log.Printf("✅ PLC 액션 토픽 구독 완료: %s", actionTopic)
	} else {
		log.Printf("❌ PLC 액션 토픽 구독 실패: %v", token.Error())
	}

	// Subscribe to robot connection status messages
	connectionTopic := "meili/v2/Roboligent/+/connection"
	token = mc.client.Subscribe(connectionTopic, mc.config.QoS, mc.handlers.RobotConnectionHandler)
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		log.Printf("✅ 로봇 연결 상태 토픽 구독 완료: %s", connectionTopic)
	} else {
		log.Printf("❌ 로봇 연결 상태 토픽 구독 실패: %v", token.Error())
	}

	// Subscribe to robot factsheet responses
	factsheetTopic := "meili/v2/+/+/factsheet"
	token = mc.client.Subscribe(factsheetTopic, mc.config.QoS, mc.handlers.RobotFactsheetHandler)
	if token.WaitTimeout(5*time.Second) && token.Error() == nil {
		log.Printf("✅ 로봇 Factsheet 토픽 구독 완료: %s", factsheetTopic)
	} else {
		log.Printf("❌ 로봇 Factsheet 토픽 구독 실패: %v", token.Error())
	}
}

// Publish publishes a message to a topic
func (mc *MQTTClient) Publish(topic string, payload []byte) error {
	if !mc.client.IsConnected() {
		return fmt.Errorf("MQTT 클라이언트가 연결되지 않음")
	}

	// Publish with timeout check
	token := mc.client.Publish(topic, mc.config.QoS, false, payload)

	// Wait for publish completion with timeout
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("MQTT 발행 타임아웃")
	}

	if token.Error() != nil {
		return fmt.Errorf("MQTT 발행 실패: %w", token.Error())
	}

	return nil
}

// StartConnectionMonitor starts monitoring connection status
func (mc *MQTTClient) StartConnectionMonitor() {
	mc.shutdownWG.Add(1)
	go func() {
		defer mc.shutdownWG.Done()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				status := mc.GetConnectionStatus()
				reconnects := atomic.LoadInt32(&mc.reconnectCount)

				log.Printf("📊 MQTT 연결 상태: %s (재연결: %d회)", status, reconnects)

				// Alert if connection is down
				if status != Connected {
					log.Printf("⚠️  MQTT 연결 이상 - 상태: %s", status)
				}

			case <-mc.connectionMonitorStop:
				return
			case <-mc.shutdownCtx.Done():
				return
			}
		}
	}()
}

// Stop gracefully disconnects the MQTT client
func (mc *MQTTClient) Stop() {
	log.Printf("🛑 MQTT 클라이언트 종료 중...")

	// Signal shutdown
	mc.shutdownCancel()

	// Stop connection monitor
	close(mc.connectionMonitorStop)

	// Disconnect client
	if mc.client.IsConnected() {
		mc.client.Disconnect(250)
		log.Printf("✅ MQTT 클라이언트 연결 해제됨")
	}

	// Wait for goroutines to finish
	mc.shutdownWG.Wait()

	log.Printf("✅ MQTT 클라이언트 종료 완료")
}

// GetReconnectCount returns the number of reconnection attempts
func (mc *MQTTClient) GetReconnectCount() int32 {
	return atomic.LoadInt32(&mc.reconnectCount)
}
