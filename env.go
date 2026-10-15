package main

import (
	"errors"
	"github.com/joho/godotenv"
)

func loadEnvFile(path string) error {
	if err := godotenv.Load(path); err != nil {
		return errors.New("could not read env file; check file existence and dotenv syntax (contents suppressed)")
	}
	return nil
}
