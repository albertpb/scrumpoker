package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv(path string) error {
	if err := godotenv.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
