package app

import (
	"os"
	"strconv"
)

// JWTConfig holds JWT authentication configuration
type JWTConfig struct {
	Secret          string
	ExpirationHours int
}

// LLMConfig holds LLM client configuration
type LLMConfig struct {
	APIKey string
	Model  string
}

// LoadJWTConfig loads JWT configuration from environment variables
func LoadJWTConfig() JWTConfig {
	secret := GetEnv("JWT_SECRET", "dev-secret-key-change-in-production")

	expirationHours, err := strconv.Atoi(GetEnv("JWT_EXPIRATION_HOURS", "24"))
	if err != nil {
		expirationHours = 24
	}

	return JWTConfig{
		Secret:          secret,
		ExpirationHours: expirationHours,
	}
}

// LoadLLMConfig loads LLM configuration from environment variables
func LoadLLMConfig() LLMConfig {
	return LLMConfig{
		APIKey: GetEnv("OPENAI_API_KEY", ""),
		Model:  GetEnv("OPENAI_MODEL", "gpt-4o"),
	}
}

// GetEnv retrieves an environment variable or returns a default value
func GetEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	return value
}
