package cli

import (
	"strings"
	"testing"

	"github.com/at-ishikawa/langner/internal/notebook"
	"github.com/stretchr/testify/assert"
)

func TestExtractContextsFromConversations(t *testing.T) {
	tests := []struct {
		name          string
		scene         *notebook.StoryScene
		expression    string
		definition    string
		expectedCount int
	}{
		{
			name: "Find expression in conversations",
			scene: &notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "A", Quote: "I saw a lunge in the gym"},
					{Speaker: "B", Quote: "The fighter made a quick LUNGE forward"},
					{Speaker: "C", Quote: "Nothing related here"},
				},
			},
			expression:    "lunge",
			definition:    "",
			expectedCount: 2,
		},
		{
			name: "Find definition in conversations",
			scene: &notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "A", Quote: "I'll sit out this round"},
					{Speaker: "B", Quote: "Why do you always sit on the bench?"},
					{Speaker: "C", Quote: "Let's play the game"},
				},
			},
			expression:    "sit",
			definition:    "sit out",
			expectedCount: 2, // Should find both "sit out" and "sit"
		},
		{
			name: "Empty conversations",
			scene: &notebook.StoryScene{
				Conversations: []notebook.Conversation{},
			},
			expression:    "test",
			definition:    "",
			expectedCount: 0,
		},
		{
			name: "No matching conversations",
			scene: &notebook.StoryScene{
				Conversations: []notebook.Conversation{
					{Speaker: "A", Quote: "Different content"},
					{Speaker: "B", Quote: "Other stuff"},
				},
			},
			expression:    "test",
			definition:    "",
			expectedCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contexts := extractContextsFromConversations(tc.scene, tc.expression, tc.definition)
			assert.Equal(t, tc.expectedCount, len(contexts))

			// Verify all contexts contain the expression or definition
			for _, ctx := range contexts {
				assert.True(t,
					strings.Contains(strings.ToLower(ctx.Context), strings.ToLower(tc.expression)) ||
						(tc.definition != "" && strings.Contains(strings.ToLower(ctx.Context), strings.ToLower(tc.definition))),
					"Context should contain expression or definition")

				// Verify usage is populated
				assert.NotEmpty(t, ctx.Usage, "Usage should be populated")

				// Verify usage is either the expression or definition
				assert.True(t,
					ctx.Usage == tc.expression || ctx.Usage == tc.definition,
					"Usage should be either expression or definition")
			}
		})
	}
}
