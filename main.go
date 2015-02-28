package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amir/raidman"
	"github.com/op/go-logging"

	"github.com/avalente/riemann-agent/modules"
)

const LOGGING_MODULE = "riemann-agent"

var log = logging.MustGetLogger(LOGGING_MODULE)

type ResQueue chan *raidman.Event

type CmdlineArgs struct {
	configFile string
	verbose    bool
	pidfile    string
}

type AppState struct {
	cmdLine       CmdlineArgs
	resChannel    *ResQueue
	drivers       *[]*Driver
	configuration *Configuration
	senderDone    *chan bool
}

func init() {
}

func parseCmdline() CmdlineArgs {
	cfgfile := flag.String("c", "config.json", "Configuration file")
	verbose := flag.Bool("v", false, "Verbose")
	pidfile := flag.String("p", "<none>", "Pid file location")
	flag.Parse()
	return CmdlineArgs{*cfgfile, *verbose, *pidfile}
}

func initializeLogging(fileName string, stringLevel string) {
	if fileName == "" {
		return
	}

	level, err := logging.LogLevel(stringLevel)
	if err != nil {
		level = logging.INFO
	}

	consoleLogging := func() {
		formatter := logging.MustStringFormatter(
			"%{color}[%{time:2006-01-02 15:04:05.000}] %{module} %{shortfile} ▶ %{level:.7s}%{color:reset} %{message}",
		)
		logging.SetFormatter(formatter)

		backend := logging.NewLogBackend(os.Stdout, "", 0)

		bl := logging.AddModuleLevel(backend)
		bl.SetLevel(level, "")

		logging.SetBackend(bl)
	}

	if fileName == "-" {
		consoleLogging()
	} else {
		file, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		defer file.Close()
		if err != nil {
			consoleLogging()
			log.Error("%v", err)
		} else {
			formatter := logging.MustStringFormatter(
				"[%{time:15:04:05.000}] %{level:.7s} %{module} %{message}",
			)
			logging.SetFormatter(formatter)

			backend := logging.NewLogBackend(file, "", 0)
			bl := logging.AddModuleLevel(backend)
			bl.SetLevel(level, "")
			logging.SetBackend(bl)
		}
	}
}

func createPidFile(file string) {
	if file != "" {
		file, err := os.Create(file)
		defer file.Close()
		if err != nil {
			log.Error("Can't create pid file: %v", err)
		} else {
			file.Write([]byte(fmt.Sprintf("%d", os.Getpid())))
		}

	}
}

func main() {
	// results channel
	resChannel := make(ResQueue, 10000)
	emptyDrivers := []*Driver{}

	state := AppState{cmdLine: parseCmdline(), resChannel: &resChannel, drivers: &emptyDrivers}

	// Wait for signal
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, os.Kill, syscall.SIGHUP, syscall.SIGTERM)
	exit := false
	for !exit {
		start(&state)

		log.Info("Instance %v started.", os.Getpid())
		createPidFile(state.configuration.PidFile)

		// Block until a signal is received.
		sig := <-sigc
		switch sig {
		case os.Interrupt, os.Kill, syscall.SIGTERM:
			exit = true
		case syscall.SIGHUP:
			log.Notice("Reloading...")
			StopAll(&state)
			continue
		default:
			log.Debug("Unhandled signal: %v", sig)
		}
	}

	log.Info("exiting")

	StopAll(&state)
	if state.configuration.PidFile != "" {
		os.Remove(state.configuration.PidFile)
	}
}

func StopAll(state *AppState) {
	*state.senderDone <- true
	StopDrivers(*state.drivers)
}

func start(state *AppState) {
	// Read configuration from file
	cfg, err := getConfiguration(state.cmdLine.configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't read configuration file: %v\n", err)
		os.Exit(1)
	}

	state.configuration = cfg

	// Override configurated pid file
	if state.cmdLine.pidfile != "<none>" {
		cfg.PidFile = state.cmdLine.pidfile
	}

	// If verbose, always log on stdout
	if state.cmdLine.verbose {
		cfg.LogFile = "-"
		cfg.LogLevel = "debug"
	}

	initializeLogging(cfg.LogFile, cfg.LogLevel)

	// create the listener
	chn := make(chan bool)
	state.senderDone = &chn
	go riemannSender(cfg, state.resChannel, &chn)

	// Start the drivers
	availableModules := modules.ScanModules(cfg.ModulesDirectory)
	log.Info("%v modules loaded", len(availableModules))

	newDrivers := GetDrivers(availableModules, cfg.DriversDirectory)
	log.Info("%v drivers loaded", len(newDrivers))

	StartDrivers(newDrivers, *state.resChannel)

	state.drivers = &newDrivers
}

func riemannConnect(cfg *Configuration, done *chan bool) *raidman.Client {
	for i := 0; i <= 10; i++ {
		conn, err := raidman.Dial(cfg.RiemannProtocol, cfg.RiemannHost)

		if err == nil {
			log.Notice("Connected to riemann on %s/%s", cfg.RiemannHost, cfg.RiemannProtocol)
			return conn
		}

		w := math.Pow(2, float64(i))
		log.Error("can't connect to riemann: %v - waiting %v seconds", err, w)
		timerChan := time.After(time.Duration(w) * time.Second)

		select {
		case <-*done:
			return nil
		case <-timerChan:
			continue
		}
	}
	panic("Can't connect to riemann")
}

func riemannSender(cfg *Configuration, channel *ResQueue, done *chan bool) {
	exit := false

	conn := riemannConnect(cfg, done)
	if conn != nil {
		defer conn.Close()
	} else {
		exit = true
	}

	for !exit {
		select {
		case <-*done:
			log.Debug("Terminating sender")
			exit = true
		case message := <-*channel:
			if message == nil {
				exit = true
			}
			err := conn.Send(message)
			if err != nil {
				log.Error("Error during send: %v", err)
				// re-enqueue the consumed message
				*channel <- message
				// reconnect
				conn = riemannConnect(cfg, done)
				if conn != nil {
					defer conn.Close()
				}
			}
		}
	}
}
