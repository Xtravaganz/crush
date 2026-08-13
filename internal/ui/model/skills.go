package model

import (
	"context"
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
	"github.com/charmbracelet/crush/internal/workflow"
)

type skillStatusItem struct {
	icon  string
	name  string
	title string
	order *int
	// description is reserved for future use (e.g. showing error details).
	description string
}

type workflowSkillActivatedMsg struct {
	entry     skills.CatalogEntry
	sessionID string
	restored  bool
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

func compareWorkflowOrder(aOrder, bOrder *int, aName, bName string) int {
	switch {
	case aOrder != nil && bOrder != nil:
		if *aOrder != *bOrder {
			return *aOrder - *bOrder
		}
	case aOrder != nil:
		return -1
	case bOrder != nil:
		return 1
	}
	return strings.Compare(strings.ToLower(aName), strings.ToLower(bName))
}

func compareWorkflowSkillEntries(a, b skills.CatalogEntry) int {
	return compareWorkflowOrder(a.WorkflowOrder, b.WorkflowOrder, a.Name, b.Name)
}

func compareSkillStatusItems(a, b skillStatusItem) int {
	return compareWorkflowOrder(a.order, b.order, a.name, b.name)
}

func (m *UI) setCyclableSkills(entries []skills.CatalogEntry) {
	m.cyclableSkills = m.cyclableSkills[:0]
	for _, entry := range entries {
		// workflow-order opts a normal user/project skill into the Tab cycle
		// and defines its position without adding skill names to Go code.
		if entry.Source == skills.SourceSystem || !entry.UserInvocable || entry.WorkflowOrder == nil {
			continue
		}
		m.cyclableSkills = append(m.cyclableSkills, entry)
	}
	slices.SortStableFunc(m.cyclableSkills, compareWorkflowSkillEntries)

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
	return m.activateWorkflowSkill(m.cyclableSkills[idx], false)
}

func (m *UI) restoreWorkflowSkill() tea.Cmd {
	if len(m.cyclableSkills) == 0 || m.initialSessionID != "" || m.continueLastSession {
		return nil
	}

	root := m.com.Workspace.WorkingDir()
	return func() tea.Msg {
		if _, _, err := workflow.EnsureLocalContext(context.Background(), root); err != nil {
			return util.NewErrorMsg(err)
		}

		state, ok, err := workflow.LoadActiveState(root)
		if err != nil {
			return util.NewErrorMsg(err)
		}
		if !ok || state.ActiveSkill == "" {
			return nil
		}

		idx := slices.IndexFunc(m.cyclableSkills, func(entry skills.CatalogEntry) bool {
			return strings.EqualFold(entry.Name, state.ActiveSkill)
		})
		if idx < 0 {
			return nil
		}
		return m.resolveWorkflowSkillActivation(m.cyclableSkills[idx], true)
	}
}

func (m *UI) activateWorkflowSkill(entry skills.CatalogEntry, restored bool) tea.Cmd {
	return func() tea.Msg {
		return m.resolveWorkflowSkillActivation(entry, restored)
	}
}

func (m *UI) resolveWorkflowSkillActivation(entry skills.CatalogEntry, restored bool) tea.Msg {
	root := m.com.Workspace.WorkingDir()
	state, ok, err := workflow.LoadActiveState(root)
	if err != nil {
		return util.NewErrorMsg(err)
	}
	if !ok {
		// Skills are still useful without an active workflow (for example
		// outside Git), so preserve Crush's normal single-session behavior.
		return workflowSkillActivatedMsg{entry: entry, restored: restored}
	}

	ctx := context.Background()
	sessionID := state.WorkerSessions[entry.Name]
	if sessionID != "" {
		if _, err := m.com.Workspace.GetSession(ctx, sessionID); err != nil {
			sessionID = ""
		}
	}
	if sessionID == "" {
		sess, err := m.com.Workspace.CreateSession(ctx, "Workflow · "+entry.Name)
		if err != nil {
			return util.NewErrorMsg(err)
		}
		sessionID = sess.ID
	}

	if err := workflow.SetWorkerSession(root, entry.Name, sessionID); err != nil {
		return util.NewErrorMsg(err)
	}
	return workflowSkillActivatedMsg{entry: entry, sessionID: sessionID, restored: restored}
}

func (m *UI) skillStatusItems() []skillStatusItem {
	t := m.com.Styles
	var items []skillStatusItem

	orderByName := make(map[string]*int, len(m.cyclableSkills))
	for _, entry := range m.cyclableSkills {
		orderByName[strings.ToLower(entry.Name)] = entry.WorkflowOrder
	}

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
			order: orderByName[strings.ToLower(name)],
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

	slices.SortStableFunc(items, compareSkillStatusItems)

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
