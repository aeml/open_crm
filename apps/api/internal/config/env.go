package config

import "os"

type Env struct {
	Port string
}

func Load() Env {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	return Env{Port: port}
}

func (e Env) APIAddress() string {
	return ":" + e.Port
}
