package modules

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/amir/raidman"
)

var HttpModule = Module{
	"http", "builtin",
	[]ModuleParameter{
		ModuleParameter{"url", "string", true, nil},
		ModuleParameter{"method", "string", false, "GET"},
		ModuleParameter{"headers", "map", false, map[string]string{}},
		ModuleParameter{"body", "string", false, nil},
		ModuleParameter{"timeout", "number", false, 10},
		ModuleParameter{"include_response", "bool", false, false}},
	ModuleCallable(HttpModuleImpl), ""}

func HttpModuleImpl(input ModuleParamList) EventList {
	var reader *strings.Reader

	if input["body"] == nil {
		reader = strings.NewReader("")
	} else {
		body := input["body"].(string)
		reader = strings.NewReader(body)
	}

	timeout := input["timeout"].(float64)
	withResponse := input["include_response"].(bool)

	ev := raidman.Event{State: "failure", Attributes: map[string]string{}}

	client := &http.Client{Timeout: time.Second * time.Duration(timeout)}
	req, err := http.NewRequest(input["method"].(string), input["url"].(string), reader)
	if err != nil {
		ev.Attributes["error"] = err.Error()
	} else {
		for k, v := range input["headers"].(map[string]interface{}) {
			req.Header.Add(k, v.(string))
		}
		startedOn := time.Now()
		res, err := client.Do(req)
		latency := time.Since(startedOn)

		ev.Metric = latency.Seconds()

		if err == nil {
			defer res.Body.Close()

			if res.StatusCode >= 300 {
				ev.Attributes["error"] = res.Status
				ev.Attributes["code"] = fmt.Sprintf("%d", res.StatusCode)
			} else {
				ev.State = "success"
				ev.Attributes["code"] = fmt.Sprintf("%d", res.StatusCode)
			}

			if withResponse {
				data, err := ioutil.ReadAll(res.Body)
				if err == nil {
					ev.Attributes["response"] = string(data)
				} else {
					ev.Attributes["response_error"] = err.Error()
				}
			}
		} else {
			ev.Attributes = map[string]string{"error": err.Error()}
		}
	}

	return NewEventList(&ev)
}
