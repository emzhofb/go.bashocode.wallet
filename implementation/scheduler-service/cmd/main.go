package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/emzhofb/gowallet/pkg/config"
	"github.com/emzhofb/gowallet/pkg/logger"
	"github.com/robfig/cron/v3"
)

func main() {
	cfg := config.LoadConfig()
	logZap := logger.NewLogger(cfg.AppEnv, "scheduler-service")
	logZap.Info("Starting Scheduler Service...")

	// Initialize cron runner
	c := cron.New()

	// Job 1: Daily Transaction CSV Report (every minute for simulation/testing)
	_, err := c.AddFunc("* * * * *", func() {
		logZap.Info("CRON JOB: Compiling daily transaction CSV report...")
		logZap.Info("CSV compilation complete. Saved to reports/daily_report.csv (simulated)")
	})
	if err != nil {
		log.Fatalf("Failed to add cron job 1: %v", err)
	}

	// Job 2: Expired tokens cleanup (every 5 minutes)
	_, err = c.AddFunc("*/5 * * * *", func() {
		logZap.Info("CRON JOB: Cleaning up expired refresh tokens and OTPs...")
		logZap.Info("Cleanup process complete (simulated)")
	})
	if err != nil {
		log.Fatalf("Failed to add cron job 2: %v", err)
	}

	c.Start()
	logZap.Info("Scheduler cron jobs started successfully.")

	// Wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logZap.Info("Stopping Scheduler Service...")
	c.Stop()
}
