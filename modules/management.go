package modules

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/amir/raidman"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("riemann-agent-modules")

type ModuleParameter struct {
	Name     string
	Type     string
	Required bool
	Default  interface{}
}

type Module struct {
	Name       string
	Kind       string
	Parameters []ModuleParameter
	Callable   ModuleCallable
	Executable string
}

type ModuleParamList map[string]interface{}
type ModuleCallable func(ModuleParamList) EventList

type EventList []*raidman.Event

func NewEventList(events ...*raidman.Event) EventList {
	return events
}

func GetBuiltinModules() []Module {
	pingModule := Module{
		"ping", "builtin",
		[]ModuleParameter{ModuleParameter{"target", "string", true, nil}},
		ModuleCallable(PingModuleImpl), ""}

	fakeModule := Module{
		"fake", "builtin",
		[]ModuleParameter{
			ModuleParameter{"attribute", "string", true, nil},
			ModuleParameter{"value1", "number", true, nil},
			ModuleParameter{"value2", "number", false, 42}},
		ModuleCallable(FakeModuleImpl), ""}

	return []Module{pingModule, fakeModule, HttpModule}
}

func GetCustomModules(directory string) []Module {
	log.Debug("Getting custom modules from %v", directory)

	files, err := ioutil.ReadDir(directory)
	if err != nil {
		log.Error("Can't read custom modules: %v\n", err)
		return []Module{}
	}

	modules := []Module{}

	for _, entry := range files {
		if entry.IsDir() {
			dir, err := filepath.Abs(filepath.Join(directory, entry.Name()))
			if err != nil {
				log.Error("Can't get absolute path for %v: %v", entry.Name(), err)
			}

			name := filepath.Base(dir)

			mod, err := ReadCustomModule(dir)
			if err != nil {
				log.Error("Can't read module %v: %v", name, err)
			} else {
				modules = append(modules, *mod)
			}
		}
	}

	return modules
}

func ReadCustomModule(directory string) (*Module, error) {
	file, err := os.Open(filepath.Join(directory, "metadata.json"))
	if err != nil {
		return nil, err
	} else {
		defer file.Close()

		decoder := json.NewDecoder(file)
		mod := Module{}
		err = decoder.Decode(&mod)
		if err != nil {
			return nil, err
		}

		var e string = ""

		switch {
		case mod.Name == "":
			e = "No name provided"
		case mod.Kind == "":
			e = "No kind provided"
		case mod.Kind != "executable":
			e = fmt.Sprintf("Invalid kind: %s", mod.Kind)
		}

		if e != "" {
			return nil, errors.New(e)
		}

		merr := checkCustomModuleParameters(&mod.Parameters)
		if merr != "" {
			return nil, errors.New(merr)
		}

		if mod.Executable == "" {
			mod.Executable = filepath.Join(directory, mod.Name)
		}

		log.Debug("Loaded custom module %v (%s)", mod.Name, mod.Kind)
		return &mod, nil
	}
}

func checkCustomModuleParameters(params *[]ModuleParameter) string {
	if len(*params) == 0 {
		return ""
	}

	names := map[string]bool{}

	for _, p := range *params {
		_, found := names[p.Name]
		if found {
			return fmt.Sprintf("duplicated parameter name <%s>", p.Name)
		}
		names[p.Name] = true

		if p.Type == "" {
			return fmt.Sprintf("no parameter type for <%s>", p.Name)
		}

		if p.Type != "bool" && p.Type != "number" && p.Type != "string" && p.Type != "map" {
			return fmt.Sprintf("bad parameter type for <%s>: %s", p.Name, p.Type)
		}

		//TODO: check that the default value is of the correct type
	}

	return ""
}

func ScanModules(modulesDir string) map[string]Module {
	builtin := GetBuiltinModules()
	custom := GetCustomModules(modulesDir)

	res := make(map[string]Module)

	for _, mod := range builtin {
		res[mod.Name] = mod
	}

	for _, mod := range custom {
		_, found := res[mod.Name]
		if found {
			log.Warning("Custom module <%s> will shadow builtin one", mod.Name)
		}
		res[mod.Name] = mod
	}

	return res
}

func PingModuleImpl(input ModuleParamList) EventList {
	target := input["target"].(string)

	event := raidman.Event{}

	startedOn := time.Now()
	_, err := net.DialTimeout("ip", target, time.Second)
	latency := time.Since(startedOn)

	event.Metric = latency.Seconds()

	if err == nil {
		event.State = "success"
	} else {
		event.State = "failure"
		event.Attributes = map[string]string{"error": err.Error()}
	}

	return NewEventList(&event)
}

func FakeModuleImpl(input ModuleParamList) EventList {
	ev1 := raidman.Event{}
	ev1.Attributes = map[string]string{"test": input["attribute"].(string)}
	ev1.State = "ok"
	ev1.Metric = input["value1"]
	ev1.Service = "value1"

	ev2 := raidman.Event{}
	ev2.Attributes = map[string]string{"test": input["attribute"].(string)}
	ev2.State = "ok"
	ev2.Metric = input["value2"]
	ev2.Service = "value2"

	return NewEventList(&ev1, &ev2)
}
