package model

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/skills"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/crush/internal/ui/util"
)

type skillStatusItem struct {
	icon  string
	name  string
	title string
	// description is reserved for future use (e.g. showing error details).
	description string
}

var builtinSkillsCache struct {
	once   sync.Once
	skills []*skills.Skill
}

func cachedBuiltinSkills() []*skills.Skill {
	builtinSkillsCache.once.Do(func() {
		builtinSkillsCache.skills = skills.DiscoverBuiltin()
	})
	return builtinSkillsCache.skills
}

// skillsInfo renders the skill discovery status section showing loaded and
// invalid skills.
func (m *UI) skillsInfo(width, maxItems int, isSection bool) string {
	t := m.com.Styles

	title := t.Resource.Heading.Render("Skills")
	if isSection {
		title = common.Section(t, title, width)
	}

	items := m.skillStatusItems()
	if len(items) == 0 {
		list := t.Resource.AdditionalText.Render("None")
		return lipgloss.NewStyle().Width(width).Render(fmt.Sprintf("%s\n\n%s", title, list))
	}

	list := skillsList(t, items, width, maxItems)
	return lipgloss.NewStyle().Width(width).Render(fmt.Sprintf("%s\n\n%s", title, list))
}

func (m *UI) setCyclableSkills(entries []skills.CatalogEntry) {
	m.cyclableSkills = m.cyclableSkills[:0]
	for _, entry := range entries {
		if entry.Source != skills.SourceProject || !entry.UserInvocable || strings.EqualFold(entry.Name, "sync") {
			continue
		}
		m.cyclableSkills = append(m.cyclableSkills, entry)
	}
	skillOrder := map[string]int{
		"issue":   0,
		"code":    1,
		"tests":   2,
		"review":  3,
		"docs":    4,
		"context": 5,
	}

	slices.SortStableFunc(m.cyclableSkills, func(a, b skills.CatalogEntry) int {
		aOrder, aKnown := skillOrder[strings.ToLower(a.Name)]
		bOrder, bKnown := skillOrder[strings.ToLower(b.Name)]

		switch {
		case aKnown && bKnown:
			if aOrder < bOrder {
				return -1
			}
			if aOrder > bOrder {
				return 1
			}
			return 0

		case aKnown:
			return -1

		case bKnown:
			return 1

		default:
			return strings.Compare(
				strings.ToLower(a.Label),
				strings.ToLower(b.Label),
			)
		}
	})
	if m.activeSkillID == "" {
		return
	}
	if !slices.ContainsFunc(m.cyclableSkills, func(entry skills.CatalogEntry) bool {
		return entry.ID == m.activeSkillID
	}) {
		m.activeSkillID = ""
		m.activeSkillName = ""
	}
}

func (m *UI) cycleActiveSkill(delta int) tea.Cmd {
	if len(m.cyclableSkills) == 0 {
		return nil
	}

	idx := slices.IndexFunc(m.cyclableSkills, func(entry skills.CatalogEntry) bool {
		return entry.ID == m.activeSkillID
	})
	if idx < 0 {
		if delta < 0 {
			idx = 0
		} else {
			idx = -1
		}
	}
	idx = (idx + delta + len(m.cyclableSkills)) % len(m.cyclableSkills)
	selected := m.cyclableSkills[idx]
	m.activeSkillID = selected.ID
	m.activeSkillName = selected.Name
	m.updateLayoutAndSize()

	return util.ReportInfo("Active skill: " + selected.Label)
}

func (m *UI) skillStatusItems() []skillStatusItem {
	t := m.com.Styles
	var items []skillStatusItem
	stateNames := make(map[string]struct{}, len(m.skillStates))

	disabledSet := make(map[string]bool)
	if m.com != nil && m.com.Workspace != nil {
		if cfg := m.com.Config(); cfg != nil {
			for _, name := range cfg.Options.DisabledSkills {
				disabledSet[name] = true
			}
		}
	}

	states := slices.Clone(m.skillStates)
	slices.SortStableFunc(states, func(a, b *skills.SkillState) int {
		return strings.Compare(a.Path, b.Path)
	})
	for _, state := range states {
		name := state.Name
		if name == "" {
			name = filepath.Base(filepath.Dir(state.Path))
		}
		if disabledSet[name] {
			continue
		}
		if _, exists := stateNames[name]; exists {
			continue
		}
		stateNames[name] = struct{}{}
		icon := t.Resource.OnlineIcon.String()
		if state.State == skills.StateError {
			icon = t.Resource.ErrorIcon.String()
		}
		displayName := name
		if state.Path == m.activeSkillID {
			displayName = "▶ " + name
		}
		items = append(items, skillStatusItem{
			icon:  icon,
			name:  name,
			title: t.Resource.Name.Render(displayName),
		})
	}

	builtin := cachedBuiltinSkills()
	slices.SortStableFunc(builtin, func(a, b *skills.Skill) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, skill := range builtin {
		if _, ok := stateNames[skill.Name]; ok {
			continue
		}
		if disabledSet[skill.Name] {
			continue
		}
		items = append(items, skillStatusItem{
			icon:  t.Resource.OnlineIcon.String(),
			name:  skill.Name,
			title: t.Resource.Name.Render(skill.Name),
		})
	}

	slices.SortStableFunc(items, func(a, b skillStatusItem) int {
		return strings.Compare(a.name, b.name)
	})

	return items
}

func skillsList(t *styles.Styles, items []skillStatusItem, width, maxItems int) string {
	if maxItems <= 0 {
		return ""
	}

	if len(items) > maxItems {
		visibleItems := items[:maxItems-1]
		remaining := len(items) - (maxItems - 1)
		items = append(visibleItems, skillStatusItem{
			name:  "more",
			title: t.Resource.AdditionalText.Render(fmt.Sprintf("…and %d more", remaining)),
		})
	}

	renderedItems := make([]string, 0, len(items))
	for _, item := range items {
		renderedItems = append(renderedItems, common.Status(t, common.StatusOpts{
			Icon:        item.icon,
			Title:       item.title,
			Description: item.description,
		}, width))
	}
	return lipgloss.JoinVertical(lipgloss.Left, renderedItems...)
}
