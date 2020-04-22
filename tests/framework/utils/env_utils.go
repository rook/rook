package utils

import (
	"os"
)


func IsEnvVarPresent(env string) bool {
	_, present := os.LookupEnv(env)
	return present
}

func GetEnvVarWithDefault(env, defaultValue string) string {
	val := os.Getenv(env)
	if val == "" {
		return defaultValue
	}
	return val
}
