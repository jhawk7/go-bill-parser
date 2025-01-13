package common

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	DBUser     string            `json:"dbUser" validate:"required"`
	DBPass     string            `json:"dbPass" validate:"required"`
	DBUrl      string            `json:"dbUrl" validate:"required"`
	DBToken    string            `json:"dbToken" validate:"required"`
	DBOrg      string            `json:"dbOrg" validate:"required"`
	DBBucket   string            `json:"dbBucket" validate:"required"`
	SubjectMap map[string]string `json:"subjectMap" validate:"required"`
	SenderMap  map[string]string `json:"senderMap" validate:"required"`
}

func GetConfig() *Config {
	var c Config
	bytes, readErr := os.ReadFile(os.Getenv("PARSER_CONFIG"))
	if readErr != nil {
		LogError(fmt.Errorf("failed to read config.json; %v", readErr), true)
		return nil
	}

	if err := json.Unmarshal(bytes, &c); err != nil {
		LogError(fmt.Errorf("failed to marshal config.json; %v", err), true)
		return nil
	}

	return &c
}
