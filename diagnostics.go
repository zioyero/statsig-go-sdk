package statsig

import (
	"sync"
	"time"
)

type DiagnosticsContext string

const (
	InitializeContext DiagnosticsContext = "initialize"
	ConfigSyncContext DiagnosticsContext = "config_sync"
)

type DiagnosticsKey string

const (
	DownloadConfigSpecsKey  DiagnosticsKey = "download_config_specs"
	BootstrapKey            DiagnosticsKey = "bootstrap"
	GetIDListSourcesKey     DiagnosticsKey = "get_id_list_sources"
	GetIDListKey            DiagnosticsKey = "get_id_list"
	OverallKey              DiagnosticsKey = "overall"
	DataStoreConfigSpecsKey DiagnosticsKey = "data_store_config_specs"
)

type DiagnosticsStep string

const (
	NetworkRequestStep DiagnosticsStep = "network_request"
	FetchStep          DiagnosticsStep = "fetch"
	ProcessStep        DiagnosticsStep = "process"
)

type DiagnosticsAction string

const (
	StartAction DiagnosticsAction = "start"
	EndAction   DiagnosticsAction = "end"
)

type diagnosticsBase struct {
	context DiagnosticsContext
	markers []marker
	mu      sync.RWMutex
}

type diagnostics struct {
	initDiagnostics *diagnosticsBase
	syncDiagnostics *diagnosticsBase
}

type marker struct {
	Key       *DiagnosticsKey    `json:"key,omitempty"`
	Step      *DiagnosticsStep   `json:"step,omitempty"`
	Action    *DiagnosticsAction `json:"action,omitempty"`
	Timestamp int64              `json:"timestamp"`
	tags
	diagnostics *diagnosticsBase
}

type tags struct {
	Success     *bool   `json:"success,omitempty"`
	StatusCode  *int    `json:"statusCode,omitempty"`
	SDKRegion   *string `json:"sdkRegion,omitempty"`
	IDListCount *int    `json:"idListCount,omitempty"`
	URL         *string `json:"url,omitempty"`
}

func newDiagnostics() *diagnostics {
	return &diagnostics{
		initDiagnostics: &diagnosticsBase{
			context: InitializeContext,
			markers: make([]marker, 0),
		},
		syncDiagnostics: &diagnosticsBase{
			context: ConfigSyncContext,
			markers: make([]marker, 0),
		},
	}
}

func (d *diagnosticsBase) logProcess(msg string) {
	var process StatsigProcess
	switch d.context {
	case InitializeContext:
		process = StatsigProcessInitialize
	case ConfigSyncContext:
		process = StatsigProcessSync
	}
	global.Logger().LogStep(process, msg)
}

func (d *diagnosticsBase) serialize() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return map[string]interface{}{
		"context": d.context,
		"markers": d.markers,
	}
}

func (d *diagnosticsBase) clearMarkers() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.markers = nil
}

/* Context */
func (d *diagnostics) initialize() *marker {
	return &marker{diagnostics: d.initDiagnostics}
}

func (d *diagnostics) configSync() *marker {
	return &marker{diagnostics: d.syncDiagnostics}
}

/* Keys */
func (m *marker) downloadConfigSpecs() *marker {
	m.Key = new(DiagnosticsKey)
	*m.Key = DownloadConfigSpecsKey
	return m
}

func (m *marker) bootstrap() *marker {
	m.Key = new(DiagnosticsKey)
	*m.Key = BootstrapKey
	return m
}

func (m *marker) getIdListSources() *marker {
	m.Key = new(DiagnosticsKey)
	*m.Key = GetIDListSourcesKey
	return m
}

func (m *marker) getIdList() *marker {
	m.Key = new(DiagnosticsKey)
	*m.Key = GetIDListKey
	return m
}

func (m *marker) overall() *marker {
	m.Key = new(DiagnosticsKey)
	*m.Key = OverallKey
	return m
}

func (m *marker) dataStoreConfigSpecs() *marker {
	m.Key = new(DiagnosticsKey)
	*m.Key = DataStoreConfigSpecsKey
	return m
}

/* Steps */
func (m *marker) networkRequest() *marker {
	m.Step = new(DiagnosticsStep)
	*m.Step = NetworkRequestStep
	return m
}

func (m *marker) fetch() *marker {
	m.Step = new(DiagnosticsStep)
	*m.Step = FetchStep
	return m
}

func (m *marker) process() *marker {
	m.Step = new(DiagnosticsStep)
	*m.Step = ProcessStep
	return m
}

/* Actions */
func (m *marker) start() *marker {
	m.Action = new(DiagnosticsAction)
	*m.Action = StartAction
	return m
}

func (m *marker) end() *marker {
	m.Action = new(DiagnosticsAction)
	*m.Action = EndAction
	return m
}

/* Tags */
func (m *marker) success(val bool) *marker {
	m.Success = new(bool)
	*m.Success = val
	return m
}

func (m *marker) statusCode(val int) *marker {
	m.StatusCode = new(int)
	*m.StatusCode = val
	return m
}

func (m *marker) sdkRegion(val string) *marker {
	m.SDKRegion = new(string)
	*m.SDKRegion = val
	return m
}

func (m *marker) idListCount(val int) *marker {
	m.IDListCount = new(int)
	*m.IDListCount = val
	return m
}

func (m *marker) url(val string) *marker {
	m.URL = new(string)
	*m.URL = val
	return m
}

/* End of chain */
func (m *marker) mark() {
	m.Timestamp = time.Now().Unix() * 1000
	m.diagnostics.mu.Lock()
	defer m.diagnostics.mu.Unlock()
	m.diagnostics.markers = append(m.diagnostics.markers, *m)
	m.logProcess()
}

func (m *marker) logProcess() {
	var msg string
	if *m.Key == OverallKey {
		if *m.Action == StartAction {
			msg = "Starting..."
		} else if *m.Action == EndAction {
			msg = "Done"
		}
	} else if *m.Key == DownloadConfigSpecsKey {
		if *m.Step == NetworkRequestStep {
			if *m.Action == StartAction {
				msg = "Loading specs from network..."
			} else if *m.Action == EndAction {
				if *m.Success {
					msg = "Done loading specs from network"
				} else {
					msg = "Failed to load specs from network"
				}
			}
		} else if *m.Step == ProcessStep {
			if *m.Action == StartAction {
				msg = "Processing specs from network..."
			} else if *m.Action == EndAction {
				if *m.Success {
					msg = "Done processing specs from network"
				} else {
					msg = "No updates to specs from network"
				}
			}
		}
	} else if *m.Key == DataStoreConfigSpecsKey {
		if *m.Step == FetchStep {
			if *m.Action == StartAction {
				msg = "Loading specs from adapter..."
			} else if *m.Action == EndAction {
				if *m.Success {
					msg = "Done loading specs from adapter"
				} else {
					msg = "Failed to load specs from adapter"
				}
			}
		} else if *m.Step == ProcessStep {
			if *m.Action == StartAction {
				msg = "Processing specs from adapter..."
			} else if *m.Action == EndAction {
				if *m.Success {
					msg = "Done processing specs from adapter"
				} else {
					msg = "No updates to specs from adapter"
				}
			}
		}
	}
	m.diagnostics.logProcess(msg)
}