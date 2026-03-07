package modelsel

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"orx/internal/config"
)

var effortOrder = []string{"", "none", "minimal", "low", "medium", "high", "xhigh"}

func nextEffort(current string) string {
	for i, e := range effortOrder {
		if e == current {
			return effortOrder[(i+1)%len(effortOrder)]
		}
	}
	return "none"
}

func supportsReasoning(params []string) bool {
	for _, p := range params {
		if p == "reasoning" {
			return true
		}
	}
	return false
}

func filterReasoningSelectedModels(models []config.SelectedModel) []config.SelectedModel {
	var result []config.SelectedModel
	for i := range models {
		if supportsReasoning(models[i].SupportedParameters) {
			result = append(result, models[i])
		}
	}
	return result
}

type reasoningTuiApp struct {
	app       *tview.Application
	models    []config.SelectedModel
	efforts   map[string]string
	confirmed bool

	modelList *tview.List
	statusBar *tview.TextView
}

func newReasoningTuiApp(models []config.SelectedModel) *reasoningTuiApp {
	efforts := make(map[string]string, len(models))
	for i := range models {
		if models[i].ExistingParams != nil && models[i].ExistingParams.Reasoning != nil {
			efforts[models[i].ID] = models[i].ExistingParams.Reasoning.Effort
		}
	}

	a := &reasoningTuiApp{
		app:     tview.NewApplication(),
		models:  models,
		efforts: efforts,
	}
	a.buildComponents()
	a.buildLayout()
	a.setupInputHandlers()
	a.updateList()
	return a
}

func (a *reasoningTuiApp) buildComponents() {
	a.modelList = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true)

	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
}

func (a *reasoningTuiApp) buildLayout() {
	listPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.modelList, 0, 1, true)
	listPanel.SetBorder(true).SetTitle(" Reasoning Effort ")

	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(listPanel, 0, 1, true).
		AddItem(a.statusBar, 1, 0, false)

	a.app.SetRoot(mainFlex, true)
}

func (a *reasoningTuiApp) setupInputHandlers() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.confirmed = false
			a.app.Stop()
			return nil
		case tcell.KeyEnter:
			a.confirmed = true
			a.app.Stop()
			return nil
		}
		return event
	})

	a.modelList.SetInputCapture(a.handleListInput)
}

func (a *reasoningTuiApp) handleListInput(event *tcell.EventKey) *tcell.EventKey {
	if event.Key() == tcell.KeyRune && event.Rune() == ' ' {
		a.cycleEffort(a.modelList.GetCurrentItem())
		return nil
	}
	return event
}

func (a *reasoningTuiApp) effortLabel(id string) string {
	e := a.efforts[id]
	if e == "" {
		return "(skip)"
	}
	return "[" + e + "]"
}

func (a *reasoningTuiApp) updateList() {
	currentIdx := a.modelList.GetCurrentItem()
	a.modelList.Clear()

	for i := range a.models {
		id := a.models[i].ID
		label := fmt.Sprintf("%-50s  effort: %s", id, a.effortLabel(id))
		a.modelList.AddItem(label, "", 0, nil)
	}

	if len(a.models) == 0 {
		return
	}
	if currentIdx >= len(a.models) {
		currentIdx = len(a.models) - 1
	}
	if currentIdx < 0 {
		currentIdx = 0
	}
	a.modelList.SetCurrentItem(currentIdx)
	a.statusBar.SetText("[yellow]Space[-] Cycle effort  [yellow]Enter[-] Done  [yellow]Esc[-] Skip all")
}

func (a *reasoningTuiApp) cycleEffort(index int) {
	if index < 0 || index >= len(a.models) {
		return
	}
	id := a.models[index].ID
	a.efforts[id] = nextEffort(a.efforts[id])
	a.updateList()
}

func (a *reasoningTuiApp) getEfforts() map[string]string {
	result := make(map[string]string)
	for id, effort := range a.efforts {
		if effort != "" {
			result[id] = effort
		}
	}
	return result
}

func (a *reasoningTuiApp) run() error {
	return a.app.Run()
}

func applyEfforts(models []config.SelectedModel, efforts map[string]string) []config.SelectedModel {
	result := make([]config.SelectedModel, len(models))
	copy(result, models)
	for i := range result {
		if effort, ok := efforts[result[i].ID]; ok {
			result[i].ReasoningEffort = effort
		}
	}
	return result
}
