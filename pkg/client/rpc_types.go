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

type IPv4StaticConfig struct {
	Address string   `json:"address,omitempty"`
	Netmask string   `json:"netmask,omitempty"`
	Gateway string   `json:"gateway,omitempty"`
	DNS     []string `json:"dns,omitempty"`
}

type IPv6StaticConfig struct {
	Prefix  string   `json:"prefix,omitempty"`
	Gateway string   `json:"gateway,omitempty"`
	DNS     []string `json:"dns,omitempty"`
}

type NetworkSettings struct {
	DHCPClient              string            `json:"dhcp_client,omitempty"`
	Hostname                string            `json:"hostname,omitempty"`
	Domain                  string            `json:"domain,omitempty"`
	HTTPProxy               string            `json:"http_proxy,omitempty"`
	IPv4Mode                string            `json:"ipv4_mode,omitempty"`
	IPv4Static              *IPv4StaticConfig `json:"ipv4_static,omitempty"`
	IPv6Mode                string            `json:"ipv6_mode,omitempty"`
	IPv6Static              *IPv6StaticConfig `json:"ipv6_static,omitempty"`
	LLDPMode                string            `json:"lldp_mode,omitempty"`
	LLDPTxTLVs              []string          `json:"lldp_tx_tlvs,omitempty"`
	MDNSMode                string            `json:"mdns_mode,omitempty"`
	TimeSyncMode            string            `json:"time_sync_mode,omitempty"`
	TimeSyncOrdering        []string          `json:"time_sync_ordering,omitempty"`
	TimeSyncDisableFallback bool              `json:"time_sync_disable_fallback,omitempty"`
	TimeSyncParallel        int               `json:"time_sync_parallel,omitempty"`
	TimeSyncNTPServers      []string          `json:"time_sync_ntp_servers,omitempty"`
	TimeSyncHTTPUrls        []string          `json:"time_sync_http_urls,omitempty"`
}

type DHCPLease struct {
	IP              string    `json:"ip,omitempty"`
	Netmask         string    `json:"netmask,omitempty"`
	DNSServers      []string  `json:"dns_servers,omitempty"`
	Broadcast       string    `json:"broadcast,omitempty"`
	Domain          string    `json:"domain,omitempty"`
	NTPServers      []string  `json:"ntp_servers,omitempty"`
	Hostname        string    `json:"hostname,omitempty"`
	Routers         []string  `json:"routers,omitempty"`
	ServerID        string    `json:"server_id,omitempty"`
	LeaseExpiry     time.Time `json:"lease_expiry,omitempty"`
	MTU             int       `json:"mtu,omitempty"`
	TTL             int       `json:"ttl,omitempty"`
	BootpNextServer string    `json:"bootp_next_server,omitempty"`
	BootpServerName string    `json:"bootp_server_name,omitempty"`
	BootpFile       string    `json:"bootp_file,omitempty"`
	DHCPClient      string    `json:"dhcp_client,omitempty"`
}

type IPv6Address struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

type NetworkState struct {
	InterfaceName string        `json:"interface_name,omitempty"`
	MACAddress    string        `json:"mac_address,omitempty"`
	IPv4          string        `json:"ipv4,omitempty"`
	IPv4Addresses []string      `json:"ipv4_addresses,omitempty"`
	IPv6          string        `json:"ipv6,omitempty"`
	IPv6Addresses []IPv6Address `json:"ipv6_addresses,omitempty"`
	IPv6LinkLocal string        `json:"ipv6_link_local,omitempty"`
	IPv6Gateway   string        `json:"ipv6_gateway,omitempty"`
	DHCPLease     *DHCPLease    `json:"dhcp_lease,omitempty"`
	Hostname      string        `json:"hostname,omitempty"`
}

type USBNetworkConfig struct {
	Enabled         bool   `json:"enabled"`
	HostPreset      string `json:"host_preset"`
	Protocol        string `json:"protocol"`
	SharingMode     string `json:"sharing_mode"`
	UplinkMode      string `json:"uplink_mode"`
	UplinkInterface string `json:"uplink_interface,omitempty"`
	IPv4SubnetCIDR  string `json:"ipv4_subnet_cidr"`
	DHCPEnabled     bool   `json:"dhcp_enabled"`
	DNSProxyEnabled bool   `json:"dns_proxy_enabled"`
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

type usbNetworkConfigRequest struct {
	Config USBNetworkConfig `json:"config"`
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
