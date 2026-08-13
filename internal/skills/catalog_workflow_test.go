package skills

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogWorkflowOrderComesFromMetadata(t *testing.T) {
	skill := &Skill{
		Name:          "code",
		UserInvocable: true,
		Metadata: map[string]string{
			"workflow-order": "20",
		},
		SkillFilePath: "/tmp/code/SKILL.md",
	}

	entries := Catalog([]*Skill{skill}, nil, "")
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].WorkflowOrder)
	require.Equal(t, 20, *entries[0].WorkflowOrder)
}

func TestCatalogInvalidWorkflowOrderDoesNotOptIn(t *testing.T) {
	skill := &Skill{
		Name:          "code",
		UserInvocable: true,
		Metadata: map[string]string{
			"workflow-order": "later",
		},
		SkillFilePath: "/tmp/code/SKILL.md",
	}

	entries := Catalog([]*Skill{skill}, nil, "")
	require.Len(t, entries, 1)
	require.Nil(t, entries[0].WorkflowOrder)
}
