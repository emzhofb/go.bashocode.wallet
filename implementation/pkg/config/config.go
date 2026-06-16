package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv         string
	Port           string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	RedisHost      string
	RedisPort      string
	RabbitMQURL    string
	MongoURI       string
	MongoDBName    string
	JWTSecret      string
	JWTAccessExp   time.Duration
	JWTRefreshExp  time.Duration
	SMTPHost       string
	SMTPPort       string
	SMTPUser       string
	SMTPPassword   string
	SMTPFrom       string
	WebhookSecret  string
}

func LoadConfig() *Config {
	return &Config{
		AppEnv:        getEnv("APP_ENV", "development"),
		Port:          getEnv("PORT", "8080"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "3306"),
		DBUser:        getEnv("DB_USER", "gowallet_user"),
		DBPassword:    getEnv("DB_PASSWORD", "gowallet_password"),
		DBName:        getEnv("DB_NAME", "gowallet"),
		RedisHost:     getEnv("REDIS_HOST", "localhost"),
		RedisPort:     getEnv("REDIS_PORT", "6379"),
		RabbitMQURL:   getEnv("RABBITMQ_URL", "amqp://gowallet_user:gowallet_password@localhost:5672/"),
		MongoURI:      getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDBName:   getEnv("MONGO_DATABASE", "gowallet_audit"),
		JWTSecret:     getEnv("JWT_SECRET", "super-secret-key"),
		JWTAccessExp:  getDurationEnv("JWT_ACCESS_EXPIRY", 15*time.Minute),
		JWTRefreshExp: getDurationEnv("JWT_REFRESH_EXPIRY", 168*time.Hour),
		SMTPHost:      getEnv("SMTP_HOST", "localhost"),
		SMTPPort:      getEnv("SMTP_PORT", "1025"),
		SMTPUser:      getEnv("SMTP_USER", ""),
		SMTPPassword:  getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:      getEnv("SMTP_FROM", "noreply@gowallet.com"),
		WebhookSecret: getEnv("WEBHOOK_SECRET_KEY", "super-secret-key-change-this"),
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if val, exists := os.LookupEnv(key); exists {
		d, err := time.ParseDuration(val)
		if err == nil {
			return d
		}
	}
	return defaultValue
}

func GetIntEnv(key string, defaultValue int) int {
	if val, exists := os.LookupEnv(key); exists {
		i, err := strconv.Atoi(val)
		if err == nil {
			return i
		}
	}
	return defaultValue
}
