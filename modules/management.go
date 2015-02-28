package modules

import (
	"net"
	"time"

	"github.com/amir/raidman"
)

type ModuleParameter struct {
	Name string
	Type string
}

type ModuleParamList map[string]interface{}
type ModuleCallable func(ModuleParamList) *raidman.Event

type Module struct {
	Name       string
	Parameters []ModuleParameter
	Callable   ModuleCallable
}

func GetBuiltinModules() []Module {
	pingModule := Module{
		"ping",
		[]ModuleParameter{ModuleParameter{"target", "string"}},
		ModuleCallable(PingModuleImpl)}

	return []Module{pingModule}
}

func ScanModules(modulesDir string) map[string]Module {
	builtin := GetBuiltinModules()

	//TODO: custom modules

	res := make(map[string]Module)

	for _, mod := range builtin {
		res[mod.Name] = mod
	}

	return res
}

func PingModuleImpl(input ModuleParamList) *raidman.Event {
	target := input["target"].(string)

	event := raidman.Event{Service: "agent"}

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

	return &event
}
