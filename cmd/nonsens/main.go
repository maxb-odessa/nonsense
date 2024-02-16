package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/maxb-odessa/nonsens/internal/config"
	"github.com/maxb-odessa/nonsens/internal/sensors"
	"github.com/maxb-odessa/nonsens/internal/server"
	"github.com/maxb-odessa/slog"

	"github.com/pborman/getopt/v2"
)

func main() {

	// get cmdline args and parse them
	help := false
	debug := 0
	configFile := os.ExpandEnv("$HOME/.local/etc/nonsens.conf")
	getopt.HelpColumn = 0
	getopt.FlagLong(&help, "help", 'h', "Show this help")
	getopt.FlagLong(&debug, "debug", 'd', "Set debug log level")
	getopt.FlagLong(&configFile, "config", 'c', "Path to config file")
	getopt.Parse()

	// help-only requested
	if help {
		getopt.Usage()
		return
	}

	// setup logger
	slog.Init("", debug, "")

	// don't run us as root
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		slog.Err("Please don't run me as root")
		return
	}

	// set proggie termination signal handler(s)
	done := make(chan bool)
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		for sig := range sigChan {
			slog.Info("Got signal '%s'", sig)
			done <- true
		}
	}()

	slog.Info("Started")
	defer slog.Info("Exited")

	conf := new(config.Config)
	if err := conf.Load(configFile); err != nil {
		slog.Fatal("Failed to load config file '%s': %s", configFile, err)
		return
	}

	// start polling sensors
	if err := sensors.Run(conf); err != nil {
		slog.Fatal("Failed to start sensors poller: %s", err)
		return
	}

	// start http server
	if err := server.Run(conf); err != nil {
		slog.Fatal("Failed to start HTTP server: %s", err)
		return
	}

	// now wait
	<-done

}
