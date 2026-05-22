package gtkui

import (
	"context"
	"fmt"
	"strconv"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/lkarlslund/jetkvm-desktop/pkg/session"
)

// SettingsMacros provides a macro library and editor for keyboard macros.
type SettingsMacros struct {
	Box *gtk.Box

	app    *Application
	macros []session.KeyboardMacro

	listBox   *gtk.ListBox
	editBox   *gtk.Box
	nameEntry *gtk.Entry

	modEntry   *gtk.Entry
	keysEntry  *gtk.Entry
	delayEntry *gtk.Entry

	stepLabel *gtk.Label

	editing   int // index into macros being edited, -1 if none
	stepIndex int
}

func NewSettingsMacros(app *Application) *SettingsMacros {
	s := &SettingsMacros{app: app, editing: -1}
	s.Box = gtk.NewBox(gtk.OrientationVertical, 8)
	s.Box.SetMarginTop(16)
	s.Box.SetMarginBottom(16)
	s.Box.SetMarginStart(16)
	s.Box.SetMarginEnd(16)

	title := gtk.NewLabel("Keyboard Macros")
	title.AddCSSClass("title-3")
	title.SetXAlign(0)
	s.Box.Append(title)

	// --- Macro list ---
	s.listBox = gtk.NewListBox()
	s.listBox.SetSelectionMode(gtk.SelectionSingle)
	scrollList := gtk.NewScrolledWindow()
	scrollList.SetChild(s.listBox)
	scrollList.SetMinContentHeight(120)
	scrollList.SetVExpand(true)
	s.Box.Append(scrollList)

	// List action buttons
	listActions := gtk.NewBox(gtk.OrientationHorizontal, 4)
	addBtn := gtk.NewButtonWithLabel("Add Macro")
	addBtn.ConnectClicked(func() { s.addMacro() })
	upBtn := gtk.NewButtonWithLabel("Up")
	upBtn.ConnectClicked(func() { s.moveMacro(-1) })
	downBtn := gtk.NewButtonWithLabel("Down")
	downBtn.ConnectClicked(func() { s.moveMacro(1) })
	dupBtn := gtk.NewButtonWithLabel("Duplicate")
	dupBtn.ConnectClicked(func() { s.duplicateMacro() })
	delBtn := gtk.NewButtonWithLabel("Delete")
	delBtn.ConnectClicked(func() { s.deleteMacro() })
	listActions.Append(addBtn)
	listActions.Append(upBtn)
	listActions.Append(downBtn)
	listActions.Append(dupBtn)
	listActions.Append(delBtn)
	s.Box.Append(listActions)

	// --- Editor panel ---
	s.editBox = gtk.NewBox(gtk.OrientationVertical, 8)

	editorTitle := gtk.NewLabel("Editor")
	editorTitle.AddCSSClass("title-4")
	editorTitle.SetXAlign(0)
	s.editBox.Append(editorTitle)

	nameRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	nameLabel := gtk.NewLabel("Name:")
	nameLabel.SetXAlign(0)
	s.nameEntry = gtk.NewEntry()
	s.nameEntry.SetHExpand(true)
	nameRow.Append(nameLabel)
	nameRow.Append(s.nameEntry)
	s.editBox.Append(nameRow)

	// Step navigator
	stepNav := gtk.NewBox(gtk.OrientationHorizontal, 4)
	s.stepLabel = gtk.NewLabel("Step 0/0")
	prevBtn := gtk.NewButtonWithLabel("Prev")
	prevBtn.ConnectClicked(func() { s.navigateStep(-1) })
	nextBtn := gtk.NewButtonWithLabel("Next")
	nextBtn.ConnectClicked(func() { s.navigateStep(1) })
	addStepBtn := gtk.NewButtonWithLabel("Add Step")
	addStepBtn.ConnectClicked(func() { s.addStep() })
	rmStepBtn := gtk.NewButtonWithLabel("Remove Step")
	rmStepBtn.ConnectClicked(func() { s.removeStep() })
	stepUpBtn := gtk.NewButtonWithLabel("Move Up")
	stepUpBtn.ConnectClicked(func() { s.moveStep(-1) })
	stepDownBtn := gtk.NewButtonWithLabel("Move Down")
	stepDownBtn.ConnectClicked(func() { s.moveStep(1) })
	stepNav.Append(prevBtn)
	stepNav.Append(s.stepLabel)
	stepNav.Append(nextBtn)
	stepNav.Append(addStepBtn)
	stepNav.Append(rmStepBtn)
	stepNav.Append(stepUpBtn)
	stepNav.Append(stepDownBtn)
	s.editBox.Append(stepNav)

	// Step fields
	modRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	modLabel := gtk.NewLabel("Modifiers:")
	modLabel.SetXAlign(0)
	s.modEntry = gtk.NewEntry()
	s.modEntry.SetHExpand(true)
	s.modEntry.SetPlaceholderText("ctrl,shift")
	modRow.Append(modLabel)
	modRow.Append(s.modEntry)
	s.editBox.Append(modRow)

	keysRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	keysLabel := gtk.NewLabel("Keys:")
	keysLabel.SetXAlign(0)
	s.keysEntry = gtk.NewEntry()
	s.keysEntry.SetHExpand(true)
	s.keysEntry.SetPlaceholderText("a,b,c")
	keysRow.Append(keysLabel)
	keysRow.Append(s.keysEntry)
	s.editBox.Append(keysRow)

	delayRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	delayLabel := gtk.NewLabel("Delay (ms):")
	delayLabel.SetXAlign(0)
	s.delayEntry = gtk.NewEntry()
	s.delayEntry.SetHExpand(true)
	s.delayEntry.SetPlaceholderText("50")
	delayRow.Append(delayLabel)
	delayRow.Append(s.delayEntry)
	s.editBox.Append(delayRow)

	// Save/Cancel
	editorActions := gtk.NewBox(gtk.OrientationHorizontal, 8)
	editorActions.SetHAlign(gtk.AlignEnd)
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() { s.cancelEdit() })
	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() { s.saveEdit() })
	editorActions.Append(cancelBtn)
	editorActions.Append(saveBtn)
	s.editBox.Append(editorActions)

	s.editBox.SetVisible(false)
	s.Box.Append(s.editBox)

	s.loadMacros()
	return s
}

func (s *SettingsMacros) loadMacros() {
	if s.app.ctrl == nil {
		return
	}
	macros, err := s.app.ctrl.GetKeyboardMacros(context.Background())
	if err != nil {
		return
	}
	s.macros = macros
	s.refreshList()
}

func (s *SettingsMacros) refreshList() {
	for {
		row := s.listBox.RowAtIndex(0)
		if row == nil {
			break
		}
		s.listBox.Remove(row)
	}
	for _, m := range s.macros {
		label := gtk.NewLabel(m.Name)
		label.SetXAlign(0)
		s.listBox.Append(label)
	}
}

func (s *SettingsMacros) selectedIndex() int {
	row := s.listBox.SelectedRow()
	if row == nil {
		return -1
	}
	return row.Index()
}

func (s *SettingsMacros) addMacro() {
	s.macros = append(s.macros, session.KeyboardMacro{
		Name:  fmt.Sprintf("Macro %d", len(s.macros)+1),
		Steps: []session.KeyboardMacroStep{{}},
	})
	s.refreshList()
	s.editing = len(s.macros) - 1
	s.stepIndex = 0
	s.openEditor()
}

func (s *SettingsMacros) moveMacro(dir int) {
	idx := s.selectedIndex()
	if idx < 0 {
		return
	}
	target := idx + dir
	if target < 0 || target >= len(s.macros) {
		return
	}
	s.macros[idx], s.macros[target] = s.macros[target], s.macros[idx]
	s.refreshList()
	s.saveMacros()
}

func (s *SettingsMacros) duplicateMacro() {
	idx := s.selectedIndex()
	if idx < 0 {
		return
	}
	dup := s.macros[idx]
	dup.Name = dup.Name + " (copy)"
	dup.ID = ""
	s.macros = append(s.macros, dup)
	s.refreshList()
	s.saveMacros()
}

func (s *SettingsMacros) deleteMacro() {
	idx := s.selectedIndex()
	if idx < 0 {
		return
	}
	s.macros = append(s.macros[:idx], s.macros[idx+1:]...)
	s.refreshList()
	s.saveMacros()
}

func (s *SettingsMacros) openEditor() {
	if s.editing < 0 || s.editing >= len(s.macros) {
		return
	}
	m := s.macros[s.editing]
	s.nameEntry.SetText(m.Name)
	s.editBox.SetVisible(true)
	s.loadStep()
}

func (s *SettingsMacros) cancelEdit() {
	s.editBox.SetVisible(false)
	s.editing = -1
	s.loadMacros()
}

func (s *SettingsMacros) saveEdit() {
	if s.editing < 0 || s.editing >= len(s.macros) {
		return
	}
	s.saveCurrentStep()
	s.macros[s.editing].Name = s.nameEntry.Text()
	s.editBox.SetVisible(false)
	s.refreshList()
	s.saveMacros()
	s.editing = -1
}

func (s *SettingsMacros) navigateStep(dir int) {
	if s.editing < 0 {
		return
	}
	s.saveCurrentStep()
	s.stepIndex += dir
	steps := s.macros[s.editing].Steps
	if s.stepIndex < 0 {
		s.stepIndex = 0
	}
	if s.stepIndex >= len(steps) {
		s.stepIndex = len(steps) - 1
	}
	s.loadStep()
}

func (s *SettingsMacros) addStep() {
	if s.editing < 0 {
		return
	}
	s.saveCurrentStep()
	s.macros[s.editing].Steps = append(s.macros[s.editing].Steps, session.KeyboardMacroStep{})
	s.stepIndex = len(s.macros[s.editing].Steps) - 1
	s.loadStep()
}

func (s *SettingsMacros) removeStep() {
	if s.editing < 0 {
		return
	}
	steps := s.macros[s.editing].Steps
	if len(steps) <= 1 {
		return
	}
	steps = append(steps[:s.stepIndex], steps[s.stepIndex+1:]...)
	s.macros[s.editing].Steps = steps
	if s.stepIndex >= len(steps) {
		s.stepIndex = len(steps) - 1
	}
	s.loadStep()
}

func (s *SettingsMacros) moveStep(dir int) {
	if s.editing < 0 {
		return
	}
	s.saveCurrentStep()
	steps := s.macros[s.editing].Steps
	target := s.stepIndex + dir
	if target < 0 || target >= len(steps) {
		return
	}
	steps[s.stepIndex], steps[target] = steps[target], steps[s.stepIndex]
	s.stepIndex = target
	s.loadStep()
}

func (s *SettingsMacros) loadStep() {
	if s.editing < 0 {
		return
	}
	steps := s.macros[s.editing].Steps
	s.stepLabel.SetText(fmt.Sprintf("Step %d/%d", s.stepIndex+1, len(steps)))
	if s.stepIndex >= 0 && s.stepIndex < len(steps) {
		step := steps[s.stepIndex]
		s.modEntry.SetText(joinStrings(step.Modifiers))
		s.keysEntry.SetText(joinStrings(step.Keys))
		s.delayEntry.SetText(strconv.Itoa(step.Delay))
	}
}

func (s *SettingsMacros) saveCurrentStep() {
	if s.editing < 0 {
		return
	}
	steps := s.macros[s.editing].Steps
	if s.stepIndex < 0 || s.stepIndex >= len(steps) {
		return
	}
	steps[s.stepIndex].Modifiers = splitComma(s.modEntry.Text())
	steps[s.stepIndex].Keys = splitComma(s.keysEntry.Text())
	delay, _ := strconv.Atoi(s.delayEntry.Text())
	steps[s.stepIndex].Delay = delay
}

func (s *SettingsMacros) saveMacros() {
	if s.app.ctrl == nil {
		return
	}
	for i := range s.macros {
		s.macros[i].SortOrder = i
	}
	_ = s.app.ctrl.SetKeyboardMacros(s.macros)
}

func joinStrings(ss []string) string {
	result := ""
	for i, v := range ss {
		if i > 0 {
			result += ","
		}
		result += v
	}
	return result
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			part := trimSpace(s[start:i])
			if part != "" {
				parts = append(parts, part)
			}
			start = i + 1
		}
	}
	part := trimSpace(s[start:])
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}

func trimSpace(s string) string {
	i, j := 0, len(s)
	for i < j && s[i] == ' ' {
		i++
	}
	for j > i && s[j-1] == ' ' {
		j--
	}
	return s[i:j]
}
