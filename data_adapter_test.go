package statsig

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBootstrapWithAdapter(t *testing.T) {
	events := []Event{}
	dcs_bytes, _ := os.ReadFile("download_config_specs.json")
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		if strings.Contains(req.URL.Path, "log_event") {
			type requestInput struct {
				Events          []Event         `json:"events"`
				StatsigMetadata statsigMetadata `json:"statsigMetadata"`
			}
			input := &requestInput{}
			defer req.Body.Close()
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(req.Body)

			_ = json.Unmarshal(buf.Bytes(), &input)
			events = input.Events
		}
	}))
	dataAdapter := dataAdapterExample{store: make(map[string]string)}
	dataAdapter.initialize()
	defer dataAdapter.shutdown()
	dataAdapter.set(dataAdapterKey, string(dcs_bytes))
	options := &Options{
		DataAdapter: dataAdapter,
		API:         testServer.URL,
		Environment: Environment{Tier: "test"},
	}
	InitializeWithOptions("secret-key", options)
	user := User{UserID: "statsig_user", Email: "statsiguser@statsig.com"}

	t.Run("able to fetch data from adapter and populate store without network", func(t *testing.T) {
		value := CheckGate(user, "always_on_gate")
		if !value {
			t.Errorf("Expected gate to return true")
		}
		config := GetConfig(user, "test_config")
		if config.GetString("string", "") != "statsig" {
			t.Errorf("Expected config to return statsig")
		}
		layer := GetLayer(user, "a_layer")
		if layer.GetString("experiment_param", "") != "control" {
			t.Errorf("Expected layer param to return control")
		}
		shutDownAndClearInstance() // shutdown here to flush event queue
		if len(events) != 3 {
			t.Errorf("Should receive exactly 3 log_event. Got %d", len(events))
		}
		for _, event := range events {
			if event.Metadata["reason"] != string(reasonDataAdapter) {
				t.Errorf("Expected init reason to be %s", reasonDataAdapter)
			}
		}
	})
}

func TestSaveToAdapter(t *testing.T) {
	bytes, _ := os.ReadFile("download_config_specs.json")
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		if strings.Contains(req.URL.Path, "download_config_specs") {
			var in *downloadConfigsInput
			_ = json.NewDecoder(req.Body).Decode(&in)
			_, _ = res.Write(bytes)
		}
	}))
	dataAdapter := dataAdapterExample{store: make(map[string]string)}
	options := &Options{
		DataAdapter: dataAdapter,
		API:         testServer.URL,
		Environment: Environment{Tier: "test"},
	}
	InitializeWithOptions("secret-key", options)
	defer shutDownAndClearInstance()

	t.Run("updates adapter with newer values from network", func(t *testing.T) {
		specString := dataAdapter.get(dataAdapterKey)
		specs := downloadConfigSpecResponse{}
		err := json.Unmarshal([]byte(specString), &specs)
		if err != nil {
			t.Errorf("Error parsing data adapter values")
		}
		if !contains_spec(specs.FeatureGates, "always_on_gate", "feature_gate") {
			t.Errorf("Expected data adapter to have downloaded gates")
		}
		if !contains_spec(specs.DynamicConfigs, "test_config", "dynamic_config") {
			t.Errorf("Expected data adapter to have downloaded configs")
		}
		if !contains_spec(specs.LayerConfigs, "a_layer", "dynamic_config") {
			t.Errorf("Expected data adapter to have downloaded layers")
		}
	})
}

func TestIncorrectlyImplementedAdapter(t *testing.T) {
	events := []Event{}
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
		if strings.Contains(req.URL.Path, "download_config_specs") {
			var in *downloadConfigsInput
			bytes, _ := os.ReadFile("download_config_specs.json")
			_ = json.NewDecoder(req.Body).Decode(&in)
			_, _ = res.Write(bytes)
		} else if strings.Contains(req.URL.Path, "log_event") {
			type requestInput struct {
				Events          []Event         `json:"events"`
				StatsigMetadata statsigMetadata `json:"statsigMetadata"`
			}
			input := &requestInput{}
			defer req.Body.Close()
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(req.Body)

			_ = json.Unmarshal(buf.Bytes(), &input)
			events = input.Events
		}
	}))
	dataAdapter := brokenDataAdapterExample{}
	options := &Options{
		DataAdapter: dataAdapter,
		API:         testServer.URL,
		Environment: Environment{Tier: "test"},
	}
	stderrLogs := swallow_stderr(func() {
		InitializeWithOptions("secret-key", options)
	})
	if stderrLogs == "" {
		t.Errorf("Expected output to stderr")
	}
	user := User{UserID: "statsig_user", Email: "statsiguser@statsig.com"}

	t.Run("recover and finish initialize if adapter panics", func(t *testing.T) {
		value := CheckGate(user, "always_on_gate")
		if !value {
			t.Errorf("Expected gate to return true")
		}
		config := GetConfig(user, "test_config")
		if config.GetString("string", "") != "statsig" {
			t.Errorf("Expected config to return statsig")
		}
		layer := GetLayer(user, "a_layer")
		if layer.GetString("experiment_param", "") != "control" {
			t.Errorf("Expected layer param to return control")
		}
		shutDownAndClearInstance() // shutdown here to flush event queue
		if len(events) != 3 {
			t.Errorf("Should receive exactly 3 log_event. Got %d", len(events))
		}
		for _, event := range events {
			if event.Metadata["reason"] != string(reasonNetwork) {
				t.Errorf("Expected init reason to be %s", reasonNetwork)
			}
		}
	})
}

func swallow_stderr(task func()) string {
	stderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	task()
	w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	os.Stderr = stderr
	return buf.String()
}

func contains_spec(specs []configSpec, name string, specType string) bool {
	for _, e := range specs {
		if e.Name == name && e.Type == specType {
			return true
		}
	}
	return false
}