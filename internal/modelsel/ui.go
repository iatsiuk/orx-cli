package modelsel

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"orx/internal/config"
)

type tuiApp struct {
	app        *tview.Application
	models     []APIModel
	filtered   []APIModel
	selected   map[string]bool
	searchText string

	searchInput *tview.InputField
	modelList   *tview.List
	detailsView *tview.TextView
	statusBar   *tview.TextView

	confirmed bool
}

func newTuiApp(models []APIModel, preSelected []string) *tuiApp {
	selected := make(map[string]bool)

	if len(preSelected) > 0 {
		available := make(map[string]bool, len(models))
		for i := range models {
			available[models[i].ID] = true
		}
		for _, id := range preSelected {
			if available[id] {
				selected[id] = true
			}
		}
	}

	a := &tuiApp{
		app:      tview.NewApplication(),
		models:   models,
		filtered: models,
		selected: selected,
	}
	a.buildComponents()
	a.buildLayout()
	a.setupInputHandlers()
	a.updateModelList()
	a.updateStatusBar()
	return a
}

func (a *tuiApp) buildComponents() {
	a.searchInput = tview.NewInputField().
		SetLabel("Search: ").
		SetFieldWidth(0).
		SetChangedFunc(func(text string) {
			a.searchText = text
			a.filterModels()
			a.updateModelList()
			a.updateStatusBar()
		})
	a.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyDown, tcell.KeyUp:
			a.app.SetFocus(a.modelList)
			return event
		}
		return event
	})

	a.modelList = tview.NewList().
		ShowSecondaryText(false).
		SetHighlightFullLine(true).
		SetChangedFunc(func(index int, mainText, secondaryText string, shortcut rune) {
			a.updateDetails(index)
		})

	a.detailsView = tview.NewTextView().
		SetDynamicColors(true).
		SetWordWrap(true).
		SetScrollable(true)
	a.detailsView.SetBorder(true).SetTitle(" Details ")

	a.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter)
}

func (a *tuiApp) buildLayout() {
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.searchInput, 1, 0, false).
		AddItem(a.modelList, 0, 1, true)
	leftPanel.SetBorder(true).SetTitle(" Models ")

	topFlex := tview.NewFlex().
		AddItem(leftPanel, 0, 1, true).
		AddItem(a.detailsView, 45, 0, false)

	mainFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(topFlex, 0, 1, true).
		AddItem(a.statusBar, 1, 0, false)

	a.app.SetRoot(mainFlex, true)
}

func (a *tuiApp) setupInputHandlers() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			a.app.Stop()
			return nil
		case tcell.KeyTab:
			a.cycleFocus()
			return nil
		case tcell.KeyEnter:
			return a.handleEnter(event)
		}

		if event.Rune() == '/' {
			a.app.SetFocus(a.searchInput)
			return nil
		}

		return event
	})

	a.modelList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			r := event.Rune()
			if r == ' ' {
				a.toggleCurrentSelection()
				return nil
			}
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				a.app.SetFocus(a.searchInput)
				a.searchInput.SetText(string(r))
				return nil
			}
		}
		return event
	})
}

func (a *tuiApp) filterModels() {
	if a.searchText == "" {
		a.filtered = a.models
		return
	}

	search := strings.ToLower(a.searchText)
	var result []APIModel
	for i := range a.models {
		if strings.Contains(strings.ToLower(a.models[i].ID), search) ||
			strings.Contains(strings.ToLower(a.models[i].Name), search) {
			result = append(result, a.models[i])
		}
	}
	a.filtered = result
}

func (a *tuiApp) updateModelList() {
	currentIdx := a.modelList.GetCurrentItem()
	a.modelList.Clear()

	if len(a.filtered) == 0 {
		a.modelList.AddItem("No models match search", "", 0, nil)
		a.detailsView.Clear()
		return
	}

	for i := range a.filtered {
		var marker string
		if a.selected[a.filtered[i].ID] {
			marker = "(x)"
		} else {
			marker = "( )"
		}
		a.modelList.AddItem(fmt.Sprintf("%s %s", marker, a.filtered[i].ID), "", 0, nil)
	}

	if currentIdx >= len(a.filtered) {
		currentIdx = len(a.filtered) - 1
	}
	if currentIdx < 0 {
		currentIdx = 0
	}
	a.modelList.SetCurrentItem(currentIdx)
	a.updateDetails(currentIdx)
}

func (a *tuiApp) updateDetails(index int) {
	if index < 0 || index >= len(a.filtered) {
		a.detailsView.Clear()
		return
	}

	m := a.filtered[index]

	var sb strings.Builder
	fmt.Fprintf(&sb, "[yellow]%s[white]\n\n", m.Name)
	fmt.Fprintf(&sb, "[gray]ID:[white] %s\n\n", m.ID)
	fmt.Fprintf(&sb, "[gray]Context:[white] %s\n\n", formatContextLength(m.ContextLength))
	sb.WriteString("[gray]Pricing:[white]\n")
	fmt.Fprintf(&sb, "  Input:  %s\n", formatPricing(m.Pricing.Prompt))
	fmt.Fprintf(&sb, "  Output: %s\n\n", formatPricing(m.Pricing.Completion))

	if m.Description != "" {
		sb.WriteString("[gray]Description:[white]\n")
		sb.WriteString(m.Description)
	}

	a.detailsView.SetText(sb.String())
}

func (a *tuiApp) updateStatusBar() {
	count := len(a.selected)
	text := fmt.Sprintf("[yellow]Space[-] Toggle  [yellow]Enter[-] Done  [yellow]/[-] Search  [yellow]Tab[-] Switch  [yellow]Esc[-] Cancel   Selected: %d", count)
	a.statusBar.SetText(text)
}

func (a *tuiApp) handleEnter(event *tcell.EventKey) *tcell.EventKey {
	switch a.app.GetFocus() {
	case a.searchInput:
		a.app.SetFocus(a.modelList)
		return nil
	case a.detailsView:
		return nil
	case a.modelList:
		if len(a.getSelectedModels()) == 0 {
			a.showWarning("Select at least one model")
			return nil
		}
		a.confirmed = true
		a.app.Stop()
		return nil
	}
	return event
}

func (a *tuiApp) cycleFocus() {
	current := a.app.GetFocus()
	switch current {
	case a.modelList, a.searchInput:
		a.app.SetFocus(a.detailsView)
	default:
		a.app.SetFocus(a.modelList)
	}
}

func (a *tuiApp) toggleCurrentSelection() {
	index := a.modelList.GetCurrentItem()
	if index < 0 || index >= len(a.filtered) {
		return
	}

	m := a.filtered[index]
	if a.selected[m.ID] {
		delete(a.selected, m.ID)
	} else {
		a.selected[m.ID] = true
	}

	a.updateModelList()
	a.updateStatusBar()
}

func (a *tuiApp) showWarning(msg string) {
	modal := tview.NewModal().
		SetText(msg).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.buildLayout()
		})

	a.app.SetRoot(modal, true)
}

func (a *tuiApp) run() error {
	return a.app.Run()
}

func (a *tuiApp) getSelectedModels() []config.SelectedModel {
	var result []config.SelectedModel
	for i := range a.models {
		if a.selected[a.models[i].ID] {
			result = append(result, config.SelectedModel{
				ID:                  a.models[i].ID,
				Name:                a.models[i].Name,
				Enabled:             true,
				SupportedParameters: a.models[i].SupportedParameters,
				DefaultParameters:   a.models[i].DefaultParameters,
			})
		}
	}
	return result
}
