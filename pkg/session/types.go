package session

import "time"

type LocalAuthMode uint8

const (
	LocalAuthModeUnknown LocalAuthMode = iota
	LocalAuthModeNoPassword
	LocalAuthModePassword
)

type TLSMode string

const (
	TLSModeUnknown    TLSMode = ""
	TLSModeDisabled   TLSMode = "disabled"
	TLSModeSelfSigned TLSMode = "self-signed"
	TLSModeCustom     TLSMode = "custom"
)

type TLSState struct {
	Mode        TLSMode
	Certificate string
	PrivateKey  string
}

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
	LocalAuthMode LocalAuthMode
	LoopbackOnly  bool
	Cloud         CloudState
	TLS           TLSState
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
	USBNetwork      *USBNetworkConfig
	USBDeviceCount  int
	DisplayRotation DisplayRotation
	Backlight       BacklightSettings
	VideoSleepMode  *VideoSleepMode
}

type IPv4StaticConfig struct {
	Address string
	Netmask string
	Gateway string
	DNS     []string
}

type IPv6StaticConfig struct {
	Prefix  string
	Gateway string
	DNS     []string
}

type NetworkSettings struct {
	DHCPClient              string
	Hostname                string
	Domain                  string
	HTTPProxy               string
	IPv4Mode                string
	IPv4Static              *IPv4StaticConfig
	IPv6Mode                string
	IPv6Static              *IPv6StaticConfig
	LLDPMode                string
	LLDPTxTLVs              []string
	MDNSMode                string
	TimeSyncMode            string
	TimeSyncOrdering        []string
	TimeSyncDisableFallback bool
	TimeSyncParallel        int
	TimeSyncNTPServers      []string
	TimeSyncHTTPUrls        []string
}

type DHCPLease struct {
	IP              string
	Netmask         string
	DNSServers      []string
	Broadcast       string
	Domain          string
	NTPServers      []string
	Hostname        string
	Routers         []string
	ServerID        string
	LeaseExpiry     time.Time
	MTU             int
	TTL             int
	BootpNextServer string
	BootpServerName string
	BootpFile       string
	DHCPClient      string
}

type IPv6Address struct {
	Address string
	Prefix  string
}

type NetworkState struct {
	InterfaceName string
	MACAddress    string
	IPv4          string
	IPv4Addresses []string
	IPv6          string
	IPv6Addresses []IPv6Address
	IPv6LinkLocal string
	IPv6Gateway   string
	DHCPLease     *DHCPLease
	Hostname      string
}

type USBNetworkConfig struct {
	Enabled         bool
	HostPreset      string
	Protocol        string
	SharingMode     string
	UplinkMode      string
	UplinkInterface string
	IPv4SubnetCIDR  string
	DHCPEnabled     bool
	DNSProxyEnabled bool
}

type PublicIP struct {
	IPAddress   string
	LastUpdated time.Time
}

type TailscalePeer struct {
	HostName     string
	DNSName      string
	TailscaleIPs []string
	Online       bool
	OS           string
}

type TailscaleStatus struct {
	Installed    bool
	Running      bool
	BackendState string
	AuthURL      string
	ControlURL   string
	Self         *TailscalePeer
	Health       []string
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

type UpdateStatus struct {
	Local                 VersionInfo
	Remote                VersionInfo
	AppUpdateAvailable    bool
	SystemUpdateAvailable bool
}

type VideoCodec string

const (
	VideoCodecUnknown VideoCodec = ""
	VideoCodecAuto    VideoCodec = "auto"
	VideoCodecH265    VideoCodec = "h265"
	VideoCodecH264    VideoCodec = "h264"
)

type BacklightSettings struct {
	MaxBrightness int
	DimAfter      int
	OffAfter      int
}

type VideoSleepMode struct {
	Enabled  bool
	Duration int
}

type VideoState struct {
	Codec VideoCodec
	EDID  string
}

type MQTTSettings struct {
	Enabled           bool
	Broker            string
	Port              int
	Username          string
	Password          string
	BaseTopic         string
	UseTLS            bool
	TLSInsecure       bool
	EnableHADiscovery bool
	EnableActions     bool
	DebounceMs        int
}

type MQTTStatus struct {
	Connected bool
	Error     string
}

type MQTTTestResult struct {
	Success bool
	Error   string
}

type AdvancedState struct {
	DevMode      *bool
	DevChannel   *bool
	LoopbackOnly *bool
	USBEmulation *bool
	SSHKey       string
	Version      VersionInfo
}

type KeyboardMacroStep struct {
	Keys      []string
	Modifiers []string
	Delay     int
}

type KeyboardMacro struct {
	ID        string
	Name      string
	Steps     []KeyboardMacroStep
	SortOrder int
}
