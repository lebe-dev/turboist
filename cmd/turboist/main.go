package main

import (
	"flag"
	"os"

	"github.com/joho/godotenv"
	"github.com/lebe-dev/turboist/internal/config"
	"github.com/lebe-dev/turboist/internal/logging"
)

const Version = "1.0.0"

func main() {
	configPath := flag.String("config", "config.yml", "path to config.yml")
	flag.Parse()

	_ = godotenv.Load()

	env, err := config.LoadEnv()
	if err != nil {
		_, _ = os.Stderr.WriteString("env error: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(env.LogLevel)

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("config error", "err", err)
		os.Exit(1)
	}

	log.Info("starting turboist",
		"version", Version,
		"bind", env.Bind,
		"baseUrl", env.BaseURL,
		"timezone", cfg.Timezone,
	)
}
