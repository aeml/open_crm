package main

import (
	"fmt"
	"strings"
)

func databaseStartupError(goEnv string, configErr, connectionErr error) error {
	if configErr != nil {
		switch strings.ToLower(strings.TrimSpace(goEnv)) {
		case "", "development", "test":
		default:
			return fmt.Errorf("configure database: %w", configErr)
		}
	}
	if connectionErr != nil {
		return fmt.Errorf("connect database: %w", connectionErr)
	}
	return nil
}
