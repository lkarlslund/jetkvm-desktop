package session

type TLSMode string

const (
	TLSModeUnknown    TLSMode = ""
	TLSModeDisabled   TLSMode = "disabled"
	TLSModeSelfSigned TLSMode = "self-signed"
)

type DisplayRotation string

const (
	DisplayRotationUnknown  DisplayRotation = ""
	DisplayRotationNormal   DisplayRotation = "270"
	DisplayRotationInverted DisplayRotation = "90"
)

type CloudState struct {
	Connected bool
	URL       string
	AppURL    string
}

type AccessState struct {
	Cloud CloudState
	TLS   TLSMode
}

type USBConfig struct {
	VendorID     string
	ProductID    string
	SerialNumber string
	Manufacturer string
	Product      string
}

type USBDevices struct {
	AbsoluteMouse bool
	RelativeMouse bool
	Keyboard      bool
	MassStorage   bool
	SerialConsole bool
	Network       bool
}

type HardwareState struct {
	USBEmulation    *bool
	USBConfig       USBConfig
	USBDevices      USBDevices
	USBDeviceCount  int
	DisplayRotation DisplayRotation
}

type NetworkSettings struct {
	Hostname string
	IP       string
}

type NetworkState struct {
	Hostname string
	IP       string
	DHCP     *bool
}

type JigglerConfig struct {
	InactivityLimitSeconds int
	JitterPercentage       int
	ScheduleCronTab        string
	Timezone               string
}

type VersionInfo struct {
	AppVersion    string
	SystemVersion string
}

type AdvancedState struct {
	DevMode      *bool
	USBEmulation *bool
	Version      VersionInfo
}
