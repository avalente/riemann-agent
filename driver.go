package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/amir/raidman"

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

func ValidateType(name string, expType string, value interface{}) *string {
	var vtype string

	if value == nil {
		return nil
	}

	switch t := value.(type) {
	case bool:
		vtype = "bool"
	case int, int64, float64, float32:
		vtype = "number"
	case string:
		vtype = "string"
	case map[string]interface{}:
		vtype = "map"
	default:
		res := fmt.Sprintf("%s (unsupported type %T)", name, t)
		return &res
	}

	if vtype != expType {
		res := fmt.Sprintf("%s (%s not %s)", name, vtype, expType)
		return &res
	}

	return nil
}

func GetParameters(drv Driver) (modules.ModuleParamList, string) {
	params := make(modules.ModuleParamList)
	notFound := make([]string, 0)
	typeErrors := make([]string, 0)

	for _, param := range drv.ModuleObject.Parameters {
		value, found := drv.Configuration[param.Name]
		if !found {
			if param.Required {
				notFound = append(notFound, param.Name)
				continue
			} else {
				value = param.Default
			}
		}

		validationError := ValidateType(param.Name, param.Type, value)
		if validationError == nil {
			params[param.Name] = value
		} else {
			typeErrors = append(typeErrors, *validationError)
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
	switch drv.ModuleObject.Kind {
	case "builtin":
		RunBuiltin(&drv, &doneChan, &queue)
	case "executable":
		RunExecutable(&drv, &doneChan, &queue)
	}
}

func RunExecutable(pdrv *Driver, pdoneChan *chan bool, pqueue *ResQueue) {
	drv := *pdrv
	doneChan := *pdoneChan
	queue := *pqueue

	paramsMap, err := GetParameters(drv)
	if err != "" {
		log.Error("Can't run driver %s: %s - DRIVER DISABLED", drv.Id, err)
		<-doneChan
	} else {
		paramsJson, _ := json.Marshal(paramsMap)

		duration := time.Duration(drv.Interval) * time.Second

		ticker := time.NewTicker(duration)

		// start external process
		cmd := exec.Command(drv.ModuleObject.Executable)
		stdin, err_stdin := cmd.StdinPipe()
		stdout, err_stdout := cmd.StdoutPipe()

		switch {
		case err_stdin != nil:
			log.Error("Can't run driver %s on custom module %s: can't get stdin (%v) - DRIVER DISABLED", drv.Id, drv.Module, err_stdin)
			<-doneChan

		case err_stdout != nil:
			log.Error("Can't run driver %s on custom module %s: can't get stdout (%v) - DRIVER DISABLED", drv.Id, drv.Module, err_stdout)
			<-doneChan
		}

		err := cmd.Start()
		if err != nil {
			log.Error("Can't run driver %s on custom module %s: %s - DRIVER DISABLED", drv.Id, drv.Module, err)
			<-doneChan
		} else {

		loop:
			for true {
				select {
				case <-doneChan:
					log.Debug("Terminating driver %v", drv.Id)
					ticker.Stop()
					stdin.Write([]byte("exit"))
					cmd.Wait()
					break loop
				case <-ticker.C:
					//TODO: check errors
					in_ := append([]byte("call "), paramsJson...)
					in_ = append(in_, '\n')
					stdin.Write(in_)

					// events count
					var count int32
					binary.Read(stdout, binary.LittleEndian, &count)

					for i := 0; i < int(count); i++ {
						var size int32
						binary.Read(stdout, binary.LittleEndian, &size)
						buf := make([]byte, size)
						n, _ := stdout.Read(buf)
						if n != int(size) {
							//TODO: check n
						}

						ev := raidman.Event{}
						ev.Description = drv.Description
						ev.Host = drv.Host
						ev.Tags = drv.Tags
						ev.Ttl = drv.Ttl
						ev.Time = time.Now().Unix()

						decoder := json.NewDecoder(bytes.NewReader(buf))
						err := decoder.Decode(&ev)
						if err != nil {
							log.Error("Can't run driver %s on custom module %s: %s - DRIVER DISABLED", drv.Id, drv.Module, err)
							<-doneChan
							break
						} else {
							ev.Service = strings.Replace(drv.Service, "%tag", ev.Service, -1)
							queue <- &ev
						}
					}
				}
			}
		}
	}
}

func RunBuiltin(pdrv *Driver, pdoneChan *chan bool, pqueue *ResQueue) {
	drv := *pdrv
	doneChan := *pdoneChan
	queue := *pqueue

	paramsMap, err := GetParameters(drv)
	if err != "" {
		log.Error("Can't run driver %s: %s - DRIVER DISABLED", drv.Id, err)
		<-doneChan
	} else {
		duration := time.Duration(drv.Interval) * time.Second

		ticker := time.NewTicker(duration)

	loop:
		for true {
			select {
			case <-ticker.C:
				for _, ev := range drv.ModuleObject.Callable(paramsMap) {
					ev.Description = drv.Description
					ev.Service = strings.Replace(drv.Service, "%tag", ev.Service, -1)
					ev.Host = drv.Host
					ev.Tags = drv.Tags
					ev.Ttl = drv.Ttl
					ev.Time = time.Now().Unix()
					queue <- ev
				}
			case <-doneChan:
				log.Debug("Terminating driver %v", drv.Id)
				ticker.Stop()
				break loop
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
