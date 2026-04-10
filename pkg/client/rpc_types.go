package client

import (
	"time"

	"github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"
)

type LocalVersion struct {
	AppVersion    string `json:"appVersion"`
	SystemVersion string `json:"systemVersion"`
}

type UpdateStatus struct {
	Local                 LocalVersion `json:"local"`
	Remote                LocalVersion `json:"remote"`
	AppUpdateAvailable    bool         `json:"appUpdateAvailable"`
	SystemUpdateAvailable bool         `json:"systemUpdateAvailable"`
}

type PublicIP struct {
	IPAddress   string    `json:"ip"`
	LastUpdated time.Time `json:"last_updated"`
}

type TailscalePeer struct {
	HostName     string   `json:"hostName"`
	DNSName      string   `json:"dnsName"`
	TailscaleIPs []string `json:"tailscaleIPs"`
	Online       bool     `json:"online"`
	OS           string   `json:"os"`
}

type TailscaleStatus struct {
	Installed    bool           `json:"installed"`
	Running      bool           `json:"running"`
	BackendState string         `json:"backendState,omitempty"`
	AuthURL      string         `json:"authURL,omitempty"`
	ControlURL   string         `json:"controlURL,omitempty"`
	Self         *TailscalePeer `json:"self,omitempty"`
	Health       []string       `json:"health,omitempty"`
}

type BacklightSettings struct {
	MaxBrightness int `json:"max_brightness"`
	DimAfter      int `json:"dim_after"`
	OffAfter      int `json:"off_after"`
}

type VideoSleepMode struct {
	Enabled  bool `json:"enabled"`
	Duration int  `json:"duration"`
}

type MQTTSettings struct {
	Enabled           bool   `json:"enabled"`
	Broker            string `json:"broker"`
	Port              int    `json:"port"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	BaseTopic         string `json:"base_topic"`
	UseTLS            bool   `json:"use_tls"`
	TLSInsecure       bool   `json:"tls_insecure"`
	EnableHADiscovery bool   `json:"enable_ha_discovery"`
	EnableActions     bool   `json:"enable_actions"`
	DebounceMs        int    `json:"debounce_ms"`
}

type MQTTStatus struct {
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

type MQTTTestResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type NetworkSettings struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

type NetworkState struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	DHCP     bool   `json:"dhcp"`
}

type CloudState struct {
	Connected bool   `json:"connected"`
	URL       string `json:"url"`
	AppURL    string `json:"appUrl"`
}

type TLSState struct {
	Mode        string `json:"mode"`
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"privateKey,omitempty"`
}

type USBConfig struct {
	VendorID     string `json:"vendor_id"`
	ProductID    string `json:"product_id"`
	SerialNumber string `json:"serial_number"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
}

type USBDevices struct {
	AbsoluteMouse bool `json:"absolute_mouse"`
	RelativeMouse bool `json:"relative_mouse"`
	Keyboard      bool `json:"keyboard"`
	MassStorage   bool `json:"mass_storage"`
	SerialConsole bool `json:"serial_console"`
	Network       bool `json:"network"`
}

type DisplayRotationState struct {
	Rotation string `json:"rotation"`
}

type DeveloperModeState struct {
	Enabled bool `json:"enabled"`
}

type keyboardMacrosRequest struct {
	Params keyboardMacrosParams `json:"params"`
}

type keyboardMacrosParams struct {
	Macros []KeyboardMacro `json:"macros"`
}

type KeyboardMacroStep struct {
	Keys      []string `json:"keys"`
	Modifiers []string `json:"modifiers"`
	Delay     int      `json:"delay"`
}

type KeyboardMacro struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Steps     []KeyboardMacroStep `json:"steps"`
	SortOrder int                 `json:"sortOrder,omitempty"`
}

type JigglerConfig struct {
	InactivityLimitSeconds int    `json:"inactivity_limit_seconds"`
	JitterPercentage       int    `json:"jitter_percentage"`
	ScheduleCronTab        string `json:"schedule_cron_tab"`
	Timezone               string `json:"timezone,omitempty"`
}

type KeysDownState struct {
	Modifier byte   `json:"modifier"`
	Keys     []byte `json:"keys"`
}

type signalingMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type offerSignalData struct {
	SD string `json:"sd"`
}

type storageUploadRequest struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

type networkSettingsRequest struct {
	Settings NetworkSettings `json:"settings"`
}

type mqttSettingsRequest struct {
	Settings MQTTSettings `json:"settings"`
}

type setTLSStateRequest struct {
	State TLSState `json:"state"`
}

type setDisplayRotationRequest struct {
	Params DisplayRotationState `json:"params"`
}

type usbDevicesRequest struct {
	Devices USBDevices `json:"devices"`
}

type setQualityRequest struct {
	Factor float64 `json:"factor"`
}

type rebootRequest struct {
	Force bool `json:"force"`
}

type enabledStateRequest struct {
	Enabled bool `json:"enabled"`
}

type jigglerConfigRequest struct {
	JigglerConfig JigglerConfig `json:"jigglerConfig"`
}

type setCodecPreferenceRequest struct {
	Codec string `json:"codec"`
}

type setEDIDRequest struct {
	EDID string `json:"edid"`
}

type setBacklightSettingsRequest struct {
	Params BacklightSettings `json:"params"`
}

type setVideoSleepModeRequest struct {
	Duration int `json:"duration"`
}

type setSSHKeyStateRequest struct {
	SSHKey string `json:"sshKey"`
}

type wheelReportRequest struct {
	WheelY int `json:"wheelY"`
}

type storageFilesResponse struct {
	Files []virtualmedia.StorageFile `json:"files"`
}
