package client

import (
	"context"
)

func (c *Client) GetDeviceID(ctx context.Context) (string, error) {
	var deviceID string
	err := c.Call(ctx, "getDeviceID", nil, &deviceID)
	return deviceID, err
}

func (c *Client) Ping(ctx context.Context) (string, error) {
	var pong string
	err := c.Call(ctx, "ping", nil, &pong)
	return pong, err
}

func (c *Client) ForceDisconnect(ctx context.Context) error {
	return c.Call(ctx, "forceDisconnect", nil, nil)
}

func (c *Client) Reboot(ctx context.Context) error {
	return c.Call(ctx, "reboot", rebootRequest{Force: false}, nil)
}

func (c *Client) GetStreamQualityFactor(ctx context.Context) (float64, error) {
	var quality float64
	err := c.Call(ctx, "getStreamQualityFactor", nil, &quality)
	return quality, err
}

func (c *Client) SetStreamQualityFactor(ctx context.Context, factor float64) error {
	return c.Call(ctx, "setStreamQualityFactor", setQualityRequest{Factor: factor}, nil)
}

func (c *Client) GetKeyboardLayout(ctx context.Context) (string, error) {
	var layout string
	err := c.Call(ctx, "getKeyboardLayout", nil, &layout)
	return layout, err
}

func (c *Client) SetKeyboardLayout(ctx context.Context, layout string) error {
	return c.Call(ctx, "setKeyboardLayout", struct {
		Layout string `json:"layout"`
	}{Layout: layout}, nil)
}

func (c *Client) GetEDID(ctx context.Context) (string, error) {
	var edid string
	err := c.Call(ctx, "getEDID", nil, &edid)
	return edid, err
}

func (c *Client) GetLocalVersion(ctx context.Context) (LocalVersion, error) {
	var version LocalVersion
	err := c.Call(ctx, "getLocalVersion", nil, &version)
	return version, err
}

func (c *Client) GetUpdateStatus(ctx context.Context) (UpdateStatus, error) {
	var status UpdateStatus
	err := c.Call(ctx, "getUpdateStatus", nil, &status)
	return status, err
}

func (c *Client) GetAutoUpdateState(ctx context.Context) (bool, error) {
	var enabled bool
	err := c.Call(ctx, "getAutoUpdateState", nil, &enabled)
	return enabled, err
}

func (c *Client) SetAutoUpdateState(ctx context.Context, enabled bool) error {
	return c.Call(ctx, "setAutoUpdateState", enabledStateRequest{Enabled: enabled}, nil)
}

func (c *Client) GetNetworkSettings(ctx context.Context) (NetworkSettings, error) {
	var settings NetworkSettings
	err := c.Call(ctx, "getNetworkSettings", nil, &settings)
	return settings, err
}

func (c *Client) SetNetworkSettings(ctx context.Context, settings NetworkSettings) error {
	return c.Call(ctx, "setNetworkSettings", networkSettingsRequest{Settings: settings}, nil)
}

func (c *Client) GetNetworkState(ctx context.Context) (NetworkState, error) {
	var state NetworkState
	err := c.Call(ctx, "getNetworkState", nil, &state)
	return state, err
}

func (c *Client) GetCloudState(ctx context.Context) (CloudState, error) {
	var state CloudState
	err := c.Call(ctx, "getCloudState", nil, &state)
	return state, err
}

func (c *Client) GetTLSState(ctx context.Context) (TLSState, error) {
	var state TLSState
	err := c.Call(ctx, "getTLSState", nil, &state)
	return state, err
}

func (c *Client) SetTLSState(ctx context.Context, mode string) error {
	return c.Call(ctx, "setTLSState", setTLSStateRequest{State: TLSState{Mode: mode}}, nil)
}

func (c *Client) GetUSBEmulationState(ctx context.Context) (bool, error) {
	var enabled bool
	err := c.Call(ctx, "getUsbEmulationState", nil, &enabled)
	return enabled, err
}

func (c *Client) SetUSBEmulationState(ctx context.Context, enabled bool) error {
	return c.Call(ctx, "setUsbEmulationState", struct {
		Enabled bool `json:"enabled"`
	}{Enabled: enabled}, nil)
}

func (c *Client) GetUSBConfig(ctx context.Context) (USBConfig, error) {
	var cfg USBConfig
	err := c.Call(ctx, "getUsbConfig", nil, &cfg)
	return cfg, err
}

func (c *Client) GetUSBDevices(ctx context.Context) (USBDevices, error) {
	var devices USBDevices
	err := c.Call(ctx, "getUsbDevices", nil, &devices)
	return devices, err
}

func (c *Client) SetUSBDevices(ctx context.Context, devices USBDevices) error {
	return c.Call(ctx, "setUsbDevices", usbDevicesRequest{Devices: devices}, nil)
}

func (c *Client) GetDisplayRotation(ctx context.Context) (DisplayRotationState, error) {
	var state DisplayRotationState
	err := c.Call(ctx, "getDisplayRotation", nil, &state)
	return state, err
}

func (c *Client) SetDisplayRotation(ctx context.Context, rotation string) error {
	return c.Call(ctx, "setDisplayRotation", setDisplayRotationRequest{
		Params: DisplayRotationState{Rotation: rotation},
	}, nil)
}

func (c *Client) GetDeveloperModeState(ctx context.Context) (DeveloperModeState, error) {
	var state DeveloperModeState
	err := c.Call(ctx, "getDevModeState", nil, &state)
	return state, err
}

func (c *Client) SetDeveloperModeState(ctx context.Context, enabled bool) error {
	return c.Call(ctx, "setDevModeState", enabledStateRequest{Enabled: enabled}, nil)
}

func (c *Client) GetJigglerState(ctx context.Context) (bool, error) {
	var enabled bool
	err := c.Call(ctx, "getJigglerState", nil, &enabled)
	return enabled, err
}

func (c *Client) SetJigglerState(ctx context.Context, enabled bool) error {
	return c.Call(ctx, "setJigglerState", enabledStateRequest{Enabled: enabled}, nil)
}

func (c *Client) GetJigglerConfig(ctx context.Context) (JigglerConfig, error) {
	var cfg JigglerConfig
	err := c.Call(ctx, "getJigglerConfig", nil, &cfg)
	return cfg, err
}

func (c *Client) SetJigglerConfig(ctx context.Context, cfg JigglerConfig) error {
	return c.Call(ctx, "setJigglerConfig", jigglerConfigRequest{JigglerConfig: cfg}, nil)
}

func (c *Client) GetKeysDownState(ctx context.Context) (KeysDownState, error) {
	var state KeysDownState
	err := c.Call(ctx, "getKeysDownState", nil, &state)
	return state, err
}

func (c *Client) sendWheelReport(ctx context.Context, delta int8) error {
	return c.Call(ctx, "wheelReport", wheelReportRequest{WheelY: int(delta)}, nil)
}
