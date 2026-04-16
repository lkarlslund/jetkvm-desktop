package app

import (
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/lkarlslund/jetkvm-desktop/pkg/ui"
)

type textBinding struct {
	ID           string
	Value        *string
	DisplayValue string
	TextSize     float64
}

func (a *App) currentTextBinding() *textBinding {
	if a.launcherOpen {
		switch a.launcherMode {
		case launcherModeBrowse:
			return &textBinding{
				ID:       "launcher_focus_input",
				Value:    &a.launcherInput,
				TextSize: 15,
			}
		case launcherModePassword:
			return &textBinding{
				ID:           "launcher_focus_password",
				Value:        &a.launcherPassword,
				DisplayValue: strings.Repeat("*", utf8.RuneCountInString(a.launcherPassword)),
				TextSize:     15,
			}
		}
	}
	if a.mediaOpen {
		switch {
		case a.mediaURLFocused:
			return &textBinding{
				ID:       "media_focus_url",
				Value:    &a.mediaURL,
				TextSize: 13,
			}
		case a.mediaUploadFocused:
			return &textBinding{
				ID:       "media_focus_upload",
				Value:    &a.mediaUploadPath,
				TextSize: 13,
			}
		}
	}
	if !a.settingsOpen {
		return nil
	}
	value := a.currentSettingsTextValue()
	if value == nil {
		return nil
	}
	binding := &textBinding{
		ID:       a.settingsInputFocus.actionID(),
		Value:    value,
		TextSize: 13,
	}
	switch a.settingsInputFocus {
	case settingsInputAccessPassword,
		settingsInputAccessConfirmPassword,
		settingsInputAccessOldPassword,
		settingsInputAccessNewPassword,
		settingsInputAccessConfirmNewPassword,
		settingsInputAccessDisablePassword:
		binding.DisplayValue = obscuredText(*value)
	}
	return binding
}

func (f settingsInputField) actionID() string {
	switch f {
	case settingsInputJigglerCron:
		return "jiggler_focus_cron"
	case settingsInputJigglerTimezone:
		return "jiggler_focus_timezone"
	case settingsInputAccessPassword:
		return "access_focus_password"
	case settingsInputAccessConfirmPassword:
		return "access_focus_confirm_password"
	case settingsInputAccessOldPassword:
		return "access_focus_old_password"
	case settingsInputAccessNewPassword:
		return "access_focus_new_password"
	case settingsInputAccessConfirmNewPassword:
		return "access_focus_confirm_new_password"
	case settingsInputAccessDisablePassword:
		return "access_focus_disable_password"
	case settingsInputAdvancedSSH:
		return "advanced_focus_ssh"
	case settingsInputUSBNetworkUplinkInterface:
		return "usb_network_focus_uplink_interface"
	case settingsInputUSBNetworkSubnetCIDR:
		return "usb_network_focus_subnet"
	case settingsInputNetworkHostname:
		return "network_focus_hostname"
	case settingsInputNetworkDomain:
		return "network_focus_domain"
	case settingsInputNetworkHTTPProxy:
		return "network_focus_http_proxy"
	case settingsInputNetworkIPv4Address:
		return "network_focus_ipv4_address"
	case settingsInputNetworkIPv4Netmask:
		return "network_focus_ipv4_netmask"
	case settingsInputNetworkIPv4Gateway:
		return "network_focus_ipv4_gateway"
	case settingsInputNetworkIPv4DNS:
		return "network_focus_ipv4_dns"
	case settingsInputNetworkIPv6Prefix:
		return "network_focus_ipv6_prefix"
	case settingsInputNetworkIPv6Gateway:
		return "network_focus_ipv6_gateway"
	case settingsInputNetworkIPv6DNS:
		return "network_focus_ipv6_dns"
	case settingsInputNetworkTimeSyncNTP:
		return "network_focus_time_sync_ntp"
	case settingsInputNetworkTimeSyncHTTP:
		return "network_focus_time_sync_http"
	case settingsInputMacroName:
		return "macro_focus_name"
	case settingsInputMacroModifiers:
		return "macro_focus_modifiers"
	case settingsInputMacroKeys:
		return "macro_focus_keys"
	case settingsInputMacroDelay:
		return "macro_focus_delay"
	case settingsInputMQTTBroker:
		return "mqtt_focus_broker"
	case settingsInputMQTTPort:
		return "mqtt_focus_port"
	case settingsInputMQTTUsername:
		return "mqtt_focus_username"
	case settingsInputMQTTPassword:
		return "mqtt_focus_password"
	case settingsInputMQTTBaseTopic:
		return "mqtt_focus_base_topic"
	case settingsInputMQTTDebounce:
		return "mqtt_focus_debounce"
	default:
		return ""
	}
}

func (a *App) uiTextBinding() *ui.TextInputBinding {
	binding := a.currentTextBinding()
	if binding == nil {
		return nil
	}
	return &ui.TextInputBinding{
		ID:           binding.ID,
		Value:        *binding.Value,
		DisplayValue: binding.DisplayValue,
		TextSize:     binding.TextSize,
	}
}

func (a *App) pointerTextBinding(id string) *textBinding {
	if a.launcherOpen {
		switch id {
		case "launcher_focus_input":
			return &textBinding{
				ID:       id,
				Value:    &a.launcherInput,
				TextSize: 15,
			}
		case "launcher_focus_password":
			return &textBinding{
				ID:           id,
				Value:        &a.launcherPassword,
				DisplayValue: strings.Repeat("*", utf8.RuneCountInString(a.launcherPassword)),
				TextSize:     15,
			}
		}
	}
	return a.currentTextBinding()
}

func (a *App) syncTextInputBinding() *textBinding {
	binding := a.currentTextBinding()
	a.textInput.Sync(a.uiTextBinding())
	return binding
}

func (a *App) decorateTextField(field ui.TextField) ui.TextField {
	field = a.textInput.Bind(field, a.uiTextBinding())
	field.OnPointerDown = func(bounds ui.Rect) {
		a.beginTextFieldPointer(field.ID, bounds, shiftPressed())
	}
	return field
}

func (a *App) currentTextFieldRect(id string) (ui.Rect, bool) {
	for _, runtime := range []*ui.Runtime{&a.launcherRuntime, &a.mediaRuntime, &a.settingsRuntime, &a.pasteRuntime} {
		if bounds, ok := runtime.Bounds(id); ok {
			return bounds, true
		}
	}
	return ui.Rect{}, false
}

func (a *App) beginTextFieldPointer(id string, fieldRect ui.Rect, shift bool) {
	binding := a.pointerTextBinding(id)
	if binding == nil || binding.ID != id {
		return
	}
	a.textInput.Sync(&ui.TextInputBinding{
		ID:           binding.ID,
		Value:        *binding.Value,
		DisplayValue: binding.DisplayValue,
		TextSize:     binding.TextSize,
	})
	x, _ := ebiten.CursorPosition()
	a.textInput.BeginPointer(*a.uiTextBinding(), fieldRect, float64(x), shift)
}

func (a *App) updateTextSelectionDrag() {
	if !a.textInput.Dragging {
		return
	}
	binding := a.uiTextBinding()
	if binding == nil || binding.ID == "" {
		a.textInput.ClearFocus()
		return
	}
	fieldRect, ok := a.currentTextFieldRect(binding.ID)
	if !ok {
		return
	}
	x, _ := ebiten.CursorPosition()
	a.textInput.UpdateDrag(*binding, fieldRect, float64(x), ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft))
}

func (a *App) syncFocusedTextInput() bool {
	binding := a.syncTextInputBinding()
	if binding == nil {
		return false
	}
	result := a.textInput.HandleInput(*a.uiTextBinding())
	if result.Changed {
		*binding.Value = result.Value
		a.markCurrentTextBindingDirty()
	}
	return result.Handled
}

func (a *App) markCurrentTextBindingDirty() {
	if a.settingsOpen {
		switch a.settingsSection {
		case sectionMouse:
			a.jigglerEditorError = ""
		case sectionAccess:
			a.accessEditor.Message = ""
			a.accessEditor.Success = false
		case sectionAdvanced:
			a.advancedSSHDirty = true
		case sectionHardware:
			a.usbNetworkEditorDirty = true
		case sectionNetwork:
			a.networkEditorDirty = true
		case sectionMacros:
			a.macroEditor.Message = ""
			a.macroEditor.Success = false
		case sectionMQTT:
			a.mqttEditorDirty = true
			a.mqttTestMessage = ""
		}
	}
	if a.mediaOpen {
		a.mediaError = ""
	}
	if a.launcherOpen {
		a.launcherError = ""
	}
}

func shiftPressed() bool {
	return ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
}
