package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	mock_cli "github.com/at-ishikawa/langner/internal/mocks/cli"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestInteractiveQuizCLI_Run(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*mock_cli.MockSession)
		cancelAfter time.Duration
		wantErr     bool
	}{
		{
			name: "Session returns error",
			setupMock: func(mockSession *mock_cli.MockSession) {
				mockSession.EXPECT().
					Session(gomock.Any()).
					Return(errors.New("mock session error")).
					Times(1)
			},
			wantErr: true,
		},
		{
			name: "Context cancelled before first session",
			setupMock: func(mockSession *mock_cli.MockSession) {
				// May or may not be called depending on timing
				mockSession.EXPECT().
					Session(gomock.Any()).
					Return(nil).
					AnyTimes()
			},
			cancelAfter: 1 * time.Millisecond,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSession := mock_cli.NewMockSession(ctrl)
			tt.setupMock(mockSession)

			cli := &InteractiveQuizCLI{
				learningNotesDir: t.TempDir(),
			}

			ctx := context.Background()
			if tt.cancelAfter > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.cancelAfter)
				defer cancel()
			}

			err := cli.Run(ctx, mockSession)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Run("ContextPropagation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		receivedContext := make(chan context.Context, 1)

		mockSession := mock_cli.NewMockSession(ctrl)
		mockSession.EXPECT().
			Session(gomock.Any()).
			DoAndReturn(func(ctx context.Context) error {
				select {
				case receivedContext <- ctx:
				default:
				}
				return errors.New("test error")
			}).
			Times(1)

		cli := &InteractiveQuizCLI{
			learningNotesDir: t.TempDir(),
		}

		_ = cli.Run(context.Background(), mockSession)

		// Verify that context was passed to session
		select {
		case ctx := <-receivedContext:
			assert.NotNil(t, ctx)
		case <-time.After(1 * time.Second):
			t.Fatal("Context was not passed to session")
		}
	})
}
