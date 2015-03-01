package modules

import (
	"net"
	"time"

	"github.com/amir/raidman"
)

type ModuleParameter struct {
	Name     string
	Type     string
	Required bool
	Default  interface{}
}

type ModuleParamList map[string]interface{}
type ModuleCallable func(ModuleParamList) EventList

type Module struct {
	Name       string
	Parameters []ModuleParameter
	Callable   ModuleCallable
}

type EventList []*raidman.Event

func NewEventList(events ...*raidman.Event) EventList {
	return events
}

func GetBuiltinModules() []Module {
	pingModule := Module{
		"ping",
		[]ModuleParameter{ModuleParameter{"target", "string", true, nil}},
		ModuleCallable(PingModuleImpl)}

	fakeModule := Module{
		"fake",
		[]ModuleParameter{
			ModuleParameter{"attribute", "string", true, nil},
			ModuleParameter{"value1", "number", true, nil},
			ModuleParameter{"value2", "number", false, 42}},
		ModuleCallable(FakeModuleImpl)}

	return []Module{pingModule, fakeModule, HttpModule}
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
