package modals

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pdfrg/rptui/internal/config"
)

type SleepTimerMsg struct {
	Duration  time.Duration
	Cancelled bool
	Closed    bool
}

type SleepTimer struct {
	styles   *config.ThemeStyles
	cursor   int
	active   bool
	duration time.Duration
}

func (s *SleepTimer) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return SleepTimerMsg{Closed: true} }

		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}

		case "down", "j":
			if s.cursor < s.numOptions()-1 {
				s.cursor++
			}

		case "enter", " ":
			if s.active && s.cursor == 7 {
				return func() tea.Msg { return SleepTimerMsg{Cancelled: true} }
			}
			if !s.active {
				dur := s.getSelectedDuration()
				if dur > 0 {
					return func() tea.Msg { return SleepTimerMsg{Duration: dur} }
				}
			}

		case "left", "h":
			s.adjustCustom(-5)

		case "right", "l":
			s.adjustCustom(5)
		}
	}
	return nil
}

func (s *SleepTimer) numOptions() int {
	if s.active {
		return 9
	}
	return 9
}

func (s *SleepTimer) adjustCustom(delta int) {
}

func (s *SleepTimer) getSelectedDuration() time.Duration {
	switch s.cursor {
	case 0:
		return 5 * time.Minute
	case 1:
		return 10 * time.Minute
	case 2:
		return 15 * time.Minute
	case 3:
		return 30 * time.Minute
	case 4:
		return 45 * time.Minute
	case 5:
		return 60 * time.Minute
	case 6:
		return 90 * time.Minute
	case 7:
		return 2 * time.Hour
	default:
		return 0
	}
}

func (s *SleepTimer) View() string {
	var b strings.Builder

	headerStyle := s.styles.AccentStyle.
		Bold(true).
		AlignHorizontal(lipgloss.Center)

	if s.active {
		remainingMins := int(s.duration.Minutes())
		headerText := fmt.Sprintf("Sleep Timer — %d min remaining", remainingMins)
		b.WriteString(headerStyle.Render(headerText))
		b.WriteString("\n\n")
	} else {
		b.WriteString(headerStyle.Render("Sleep Timer"))
		b.WriteString("\n\n")
	}

	options := []string{"5 min", "10 min", "15 min", "30 min", "45 min", "60 min", "90 min", "2 hours", "Cancel"}
	if s.active {
		options[6] = "Change:"
		options[7] = "Cancel"
	}

	for i, label := range options {
		style := s.styles.MutedStyle

		if i == s.cursor {
			style = s.styles.AccentStyle.Bold(true)
		}

		prefix := "  "
		if i == s.cursor {
			prefix = "» "
		}

		if label == "" {
			label = "─"
		}

		b.WriteString(style.Render(prefix + label))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(s.styles.MutedStyle.Render("  [Enter] set/cancel  [Esc] close"))
	b.WriteString("\n")

	return b.String()
}

func NewSleepTimer(styles *config.ThemeStyles, active bool, dur time.Duration) *SleepTimer {
	return &SleepTimer{
		styles:   styles,
		active:   active,
		duration: dur,
		cursor:   0,
	}
}
