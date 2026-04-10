package client

import "github.com/lkarlslund/jetkvm-desktop/pkg/virtualmedia"

type LocalVersion struct {
	AppVersion    string `json:"appVersion"`
	SystemVersion string `json:"systemVersion"`
}

type UpdateStatus struct {
	AppUpdateAvailable    bool `json:"appUpdateAvailable"`
	SystemUpdateAvailable bool `json:"systemUpdateAvailable"`
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
	Mode string `json:"mode"`
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

type wheelReportRequest struct {
	WheelY int `json:"wheelY"`
}

type storageFilesResponse struct {
	Files []virtualmedia.StorageFile `json:"files"`
}
