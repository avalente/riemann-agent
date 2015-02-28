package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avalente/riemann-agent/modules"
)

type Driver struct {
	Id            string
	Description   string
	Module        string
	ModuleObject  modules.Module
	Interval      int
	Host          string
	Service       string
	Tags          []string
	Ttl           float32
	Configuration map[string]interface{}
	doneChan      chan bool
}

func StopDrivers(drivers []*Driver) {
	for _, driver := range drivers {
		driver.doneChan <- true
	}
}

func StartDrivers(drivers []*Driver, queue ResQueue) {
	for _, driver := range drivers {
		driver.doneChan = make(chan bool)
		go RunDriver(*driver, driver.doneChan, queue)
	}
}

func GetParameters(drv Driver) (modules.ModuleParamList, string) {
	params := make(modules.ModuleParamList)
	notFound := make([]string, 0)
	typeErrors := make([]string, 0)

	for _, param := range drv.ModuleObject.Parameters {
		value, found := drv.Configuration[param.Name]
		if !found {
			notFound = append(notFound, param.Name)
		} else {
			var vtype string

			switch t := value.(type) {
			case bool:
				vtype = "bool"
			case float64:
				vtype = "float"
			case int, int64:
				vtype = "int"
			case string:
				vtype = "string"
			default:
				typeErrors = append(typeErrors, fmt.Sprintf("%s (unsupported type %t)", param.Name, t))
				continue
			}

			if vtype != param.Type {
				typeErrors = append(typeErrors, fmt.Sprintf("%s (not %s)", param.Name, param.Type))
			} else {
				params[param.Name] = value
			}
		}
	}

	s1 := ""
	s2 := ""
	err := ""

	if len(notFound) > 0 {
		s1 = "required parameters not found: " + strings.Join(notFound, ", ")
	}

	if len(typeErrors) > 0 {
		s2 = "parameters with bad type: " + strings.Join(typeErrors, ", ")
	}

	if s1 != "" || s2 != "" {
		err = strings.Join([]string{s1, s2}, "; ")
	}

	return params, err
}

func RunDriver(drv Driver, doneChan chan bool, queue ResQueue) {
	paramsMap, err := GetParameters(drv)
	if err != "" {
		log.Error("Can't run driver %s: %s - DRIVER DISABLED", drv.Id, err)
		<-doneChan
	} else {
		duration := time.Duration(drv.Interval) * time.Second

		ticker := time.NewTicker(duration)

		exit := false

		for !exit {
			select {
			case <-ticker.C:
				ev := drv.ModuleObject.Callable(paramsMap)
				ev.Service = drv.Service
				ev.Host = drv.Host
				ev.Tags = drv.Tags
				ev.Ttl = drv.Ttl
				queue <- ev
			case <-doneChan:
				log.Debug("Terminating driver %v", drv.Id)
				ticker.Stop()
				exit = true
			}
		}
	}
}

func GetDrivers(availableModules map[string]modules.Module, directory string) []*Driver {
	log.Debug("Getting drivers from %v", directory)

	var err error

	files, err := ioutil.ReadDir(directory)
	if err != nil {
		log.Critical("Can't read drivers: %v\n", err)
		return []*Driver{}
	}

	drivers := []*Driver{}

	for _, entry := range files {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			name := entry.Name()
			fullName := filepath.Join(directory, name)

			log.Debug("Loading driver %s", name)

			doLog := func(reason interface{}) {
				log.Warning("Can't load driver <%v>: %v", fullName, reason)
			}

			file, err := os.Open(fullName)
			if err != nil {
				doLog(err)
			} else {
				defer file.Close()

				decoder := json.NewDecoder(file)
				drv := Driver{Interval: 30}
				err = decoder.Decode(&drv)
				if err != nil {
					doLog(err)
				} else {
					if drv.Description == "" {
						doLog("missing description")
						continue
					}

					if drv.Module == "" {
						doLog("missing module")
						continue
					}

					mod, found := availableModules[drv.Module]
					if !found {
						doLog(fmt.Sprintf("unknown module: %v", drv.Module))
						continue
					} else {
						drv.ModuleObject = mod
					}

					// override in case the attribute "id" was in the json file
					drv.Id = fullName

					// some default values
					if drv.Service == "" {
						drv.Service = drv.Description
					}
					drivers = append(drivers, &drv)
				}
			}
		}
	}

	return drivers
}
