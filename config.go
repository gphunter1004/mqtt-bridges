package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	App  AppConfig
	MQTT MQTTConfig
}

// AppConfig holds application-specific configuration
type AppConfig struct {
	Environment           string
	LogLevel              string
	StatusIntervalSeconds int
	GracefulShutdownSec   int
	TargetRobotSerials    []string // 관리 대상 로봇 시리얼 번호 목록
	AutoInitOnConnect     bool     // 로봇 연결 시 자동 초기화 여부
	AutoInitDelaySec      int      // 자동 초기화 지연 시간 (초)
	AutoFactsheetRequest  bool     // 초기화 후 자동 Factsheet 요청 여부
}

// MQTTConfig holds MQTT broker configuration (single client for bridge)
type MQTTConfig struct {
	BrokerURL            string
	ClientID             string
	Username             string
	Password             string
	QoS                  byte
	KeepAlive            int
	ConnectTimeout       int
	ReconnectDelay       int
	MaxReconnectDelay    int
	MaxReconnectAttempts int
	CleanSession         bool
}

// LoadConfig loads configuration from environment variables and .env file
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// .env file is optional, so we just log if it's not found
		fmt.Printf("Warning: .env file not found: %v\n", err)
	}

	config := &Config{
		App:  loadAppConfig(),
		MQTT: loadMQTTConfig(),
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// loadAppConfig loads application configuration
func loadAppConfig() AppConfig {
	return AppConfig{
		Environment:           getEnvString("APP_ENVIRONMENT", "development"),
		LogLevel:              getEnvString("APP_LOG_LEVEL", "info"),
		StatusIntervalSeconds: getEnvInt("APP_STATUS_INTERVAL_SECONDS", 30),
		GracefulShutdownSec:   getEnvInt("APP_GRACEFUL_SHUTDOWN_SEC", 10),
		TargetRobotSerials:    getEnvStringArray("APP_TARGET_ROBOT_SERIALS", []string{"DEX0001", "DEX0002", "DEX0003"}),
		AutoInitOnConnect:     getEnvBool("APP_AUTO_INIT_ON_CONNECT", true),
		AutoInitDelaySec:      getEnvInt("APP_AUTO_INIT_DELAY_SEC", 2),
		AutoFactsheetRequest:  getEnvBool("APP_AUTO_FACTSHEET_REQUEST", true),
	}
}

// loadMQTTConfig loads MQTT configuration (single client)
func loadMQTTConfig() MQTTConfig {
	return MQTTConfig{
		BrokerURL:            getEnvString("MQTT_BROKER_URL", "tcp://localhost:1883"),
		ClientID:             getEnvString("MQTT_CLIENT_ID", "mqtt_robot_bridge"),
		Username:             getEnvString("MQTT_USERNAME", ""),
		Password:             getEnvString("MQTT_PASSWORD", ""),
		QoS:                  byte(getEnvInt("MQTT_QOS", 1)),
		KeepAlive:            getEnvInt("MQTT_KEEP_ALIVE", 60),
		ConnectTimeout:       getEnvInt("MQTT_CONNECT_TIMEOUT", 10),
		ReconnectDelay:       getEnvInt("MQTT_RECONNECT_DELAY", 5),
		MaxReconnectDelay:    getEnvInt("MQTT_MAX_RECONNECT_DELAY", 60),
		MaxReconnectAttempts: getEnvInt("MQTT_MAX_RECONNECT_ATTEMPTS", 10),
		CleanSession:         getEnvBool("MQTT_CLEAN_SESSION", true),
	}
}

// validateConfig validates the loaded configuration
func validateConfig(config *Config) error {
	// Validate App config
	if config.App.StatusIntervalSeconds < 1 {
		return fmt.Errorf("APP_STATUS_INTERVAL_SECONDS must be greater than 0")
	}
	if len(config.App.TargetRobotSerials) == 0 {
		return fmt.Errorf("APP_TARGET_ROBOT_SERIALS must contain at least one robot serial")
	}

	// Validate MQTT config
	if config.MQTT.BrokerURL == "" {
		return fmt.Errorf("MQTT_BROKER_URL is required")
	}
	if config.MQTT.ClientID == "" {
		return fmt.Errorf("MQTT_CLIENT_ID is required")
	}
	if config.MQTT.QoS > 2 {
		return fmt.Errorf("MQTT_QOS must be 0, 1, or 2")
	}
	if config.MQTT.ConnectTimeout < 1 {
		return fmt.Errorf("MQTT_CONNECT_TIMEOUT must be greater than 0")
	}
	if config.MQTT.MaxReconnectAttempts < 1 {
		return fmt.Errorf("MQTT_MAX_RECONNECT_ATTEMPTS must be greater than 0")
	}

	return nil
}

// getEnvString gets environment variable as string with default value
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets environment variable as int with default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
		fmt.Printf("Warning: Invalid integer value for %s: %s, using default: %d\n", key, value, defaultValue)
	}
	return defaultValue
}

// getEnvBool gets environment variable as bool with default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
		fmt.Printf("Warning: Invalid boolean value for %s: %s, using default: %t\n", key, value, defaultValue)
	}
	return defaultValue
}

// getEnvStringArray gets environment variable as string array with default value
// Supports comma, semicolon, or pipe separated values
func getEnvStringArray(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	// Try different separators
	var items []string

	// Check for comma separator
	if strings.Contains(value, ",") {
		items = strings.Split(value, ",")
	} else if strings.Contains(value, ";") {
		// Check for semicolon separator
		items = strings.Split(value, ";")
	} else if strings.Contains(value, "|") {
		// Check for pipe separator
		items = strings.Split(value, "|")
	} else {
		// Single value
		items = []string{value}
	}

	// Trim whitespace from each item and filter empty values
	var result []string
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		fmt.Printf("Warning: Empty array for %s, using default\n", key)
		return defaultValue
	}

	return result
}
