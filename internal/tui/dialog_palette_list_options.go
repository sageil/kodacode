package tui

import (
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

func (d *commandPaletteDialog) listOptions() []commandPaletteListOption {
	query := strings.TrimSpace(d.filter.Value())
	switch d.kind {
	case commandPaletteModel, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return d.modelOptions(query)
	case commandPaletteAgent:
		return d.agentOptions(query)
	default:
		if query == "" {
			return d.actionOptionsEmpty()
		}
		return d.actionOptionsFiltered(query)
	}
}

func (d *commandPaletteDialog) modelOptions(query string) []commandPaletteListOption {
	if query == "" {
		return d.modelOptionsEmpty()
	}
	return d.modelOptionsFiltered(query)
}

func (d *commandPaletteDialog) modelOptionsEmpty() []commandPaletteListOption {
	options := make([]commandPaletteListOption, 0, len(d.modelItems)+1)
	seen := map[string]struct{}{}
	if option, ok := d.currentModelListOption(); ok {
		options = append(options, d.decorateMutableListOption(option))
		seen[option.Model.Ref.String()] = struct{}{}
	}
	for _, item := range d.modelItems {
		key := item.Ref.String()
		if _, ok := seen[key]; ok {
			continue
		}
		options = append(options, d.decorateMutableListOption(commandPaletteListOption{
			Label: item.ProviderName + " / " + item.ModelName,
			Model: item,
		}))
	}
	return options
}

func (d *commandPaletteDialog) modelOptionsFiltered(query string) []commandPaletteListOption {
	type scored struct {
		opt   commandPaletteListOption
		score int
	}
	matches := make([]scored, 0, len(d.modelItems)+1)
	for _, item := range d.modelItems {
		label := item.ProviderName + " / " + item.ModelName
		if ok, score := fuzzyScore(query, label+" "+item.Ref.String()+" model"); ok {
			matches = append(matches, scored{
				opt: d.decorateMutableListOption(commandPaletteListOption{
					Label: label,
					Model: item,
				}),
				score: score,
			})
		}
	}
	if exact, err := provider.ParseModelRef(query); err == nil {
		matches = append(matches, scored{
			opt: d.decorateMutableListOption(commandPaletteListOption{
				Label:       exact.ProviderID + " / " + exact.ModelID,
				Description: "(exact)",
				Model: modelItem{
					Ref:          exact,
					ProviderName: exact.ProviderID,
					ModelName:    exact.ModelID,
					Exact:        true,
				},
			}),
			score: 10_000,
		})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })

	seen := map[string]struct{}{}
	result := make([]commandPaletteListOption, 0, len(matches))
	for _, match := range matches {
		key := match.opt.Model.Ref.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, match.opt)
	}
	return result
}

func (d *commandPaletteDialog) agentOptions(query string) []commandPaletteListOption {
	if query == "" {
		return d.agentOptionsEmpty()
	}
	return d.agentOptionsFiltered(query)
}

func (d *commandPaletteDialog) agentOptionsEmpty() []commandPaletteListOption {
	options := make([]commandPaletteListOption, 0, len(d.agentItems)+1)
	seen := map[string]struct{}{}
	for _, item := range d.agentItems {
		if item.ID != d.currentAgent {
			continue
		}
		options = append(options, d.decorateMutableListOption(commandPaletteListOption{
			Label:       item.ID,
			Description: pickFirstNonBlank(item.Description, "current agent"),
			Agent:       item,
		}))
		seen[item.ID] = struct{}{}
		break
	}
	for _, item := range d.agentItems {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		options = append(options, d.decorateMutableListOption(commandPaletteListOption{
			Label:       item.ID,
			Description: item.Description,
			Agent:       item,
		}))
	}
	return options
}

func (d *commandPaletteDialog) agentOptionsFiltered(query string) []commandPaletteListOption {
	type scored struct {
		opt   commandPaletteListOption
		score int
	}
	matches := make([]scored, 0, len(d.agentItems))
	for _, item := range d.agentItems {
		if ok, score := fuzzyScore(query, item.ID+" "+item.Description+" agent"); ok {
			matches = append(matches, scored{
				opt: d.decorateMutableListOption(commandPaletteListOption{
					Label:       item.ID,
					Description: item.Description,
					Agent:       item,
				}),
				score: score,
			})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	result := make([]commandPaletteListOption, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		if _, ok := seen[match.opt.Agent.ID]; ok {
			continue
		}
		seen[match.opt.Agent.ID] = struct{}{}
		result = append(result, match.opt)
	}
	return result
}

func (d *commandPaletteDialog) currentModelListOption() (commandPaletteListOption, bool) {
	current := strings.TrimSpace(d.currentModel)
	if current == "" {
		return commandPaletteListOption{}, false
	}
	for _, item := range d.modelItems {
		if item.Ref.String() != current {
			continue
		}
		return commandPaletteListOption{
			Label:       item.ProviderName + " / " + item.ModelName,
			Description: d.currentModelDescription(),
			Model:       item,
		}, true
	}
	exact, err := provider.ParseModelRef(current)
	if err != nil {
		return commandPaletteListOption{}, false
	}
	return commandPaletteListOption{
		Label:       providerDisplayName(exact.ProviderID) + " / " + exact.ModelID,
		Description: d.currentModelDescription(),
		Model: modelItem{
			Ref:          exact,
			ProviderName: providerDisplayName(exact.ProviderID),
			ModelName:    exact.ModelID,
			Exact:        true,
		},
	}, true
}

func (d *commandPaletteDialog) currentModelDescription() string {
	switch d.kind {
	case commandPaletteUtilityModel:
		return "current utility model"
	case commandPaletteReviewerModel:
		return "current reviewer model"
	}
	return "current model"
}

func (d *commandPaletteDialog) actionOptionsEmpty() []commandPaletteListOption {
	options := make([]commandPaletteListOption, 0, len(d.actions))
	for _, action := range d.actions {
		options = append(options, d.decorateMutableListOption(commandPaletteListOption{
			Label:       action.Title,
			Description: action.Description,
			Action:      action,
		}))
	}
	return options
}

func (d *commandPaletteDialog) actionOptionsFiltered(query string) []commandPaletteListOption {
	type scored struct {
		opt   commandPaletteListOption
		score int
	}
	var matches []scored
	for _, action := range d.actions {
		if ok, score := fuzzyScore(query, action.Title+" "+action.Description+" command action"); ok {
			matches = append(matches, scored{
				opt: d.decorateMutableListOption(commandPaletteListOption{
					Label:       action.Title,
					Description: action.Description,
					Action:      action,
				}),
				score: score,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })

	result := make([]commandPaletteListOption, 0, len(matches))
	for _, match := range matches {
		result = append(result, match.opt)
	}
	return result
}

func (d *commandPaletteDialog) decorateMutableListOption(option commandPaletteListOption) commandPaletteListOption {
	if !d.listOptionSelectionLocked(option) {
		return option
	}
	option.Disabled = true
	option.Description = appendPaletteDescription(option.Description, "locked while turn runs")
	return option
}

func (d *commandPaletteDialog) mutableSelectionLocked() bool {
	return !d.allowMutableSelect
}

func (d *commandPaletteDialog) listOptionSelectionLocked(option commandPaletteListOption) bool {
	if !d.mutableSelectionLocked() {
		return false
	}
	switch d.kind {
	case commandPaletteAgent, commandPaletteModel, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return true
	case commandPaletteActions:
		switch option.Action.ID {
		case "select-model", "select-agent", "manage-sessions", "timeline", "new-session", "select-utility-model", "unset-utility-model", "select-reviewer-model", "unset-reviewer-model":
			return true
		}
	}
	return false
}

func appendPaletteDescription(base, extra string) string {
	base = strings.TrimSpace(base)
	extra = strings.TrimSpace(extra)
	switch {
	case base == "":
		return extra
	case extra == "":
		return base
	default:
		return base + " • " + extra
	}
}
