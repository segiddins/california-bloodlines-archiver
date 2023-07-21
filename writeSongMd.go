package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type writeSongMdModel struct {
	written, toWrite int

	progress progress.Model
}

// Init implements tea.Model.
func (writeSongMdModel) Init() tea.Cmd {
	panic("unimplemented")
}

// Update implements tea.Model.
func (m writeSongMdModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case songMdWrittenMsg:
		m.written++
		cmd = m.progress.SetPercent(float64(m.written) / float64(m.toWrite))
		return m, tea.Batch(cmd)
	case progress.FrameMsg:
		prog, cmd := m.progress.Update(msg)
		m.progress = prog.(progress.Model)
		return m, cmd
	default:
		_ = msg
	case errMsg:
		return ErrorModel{error: msg.error}, nil
	}

	if m.written == m.toWrite {
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model.
func (m writeSongMdModel) View() string {
	if m.toWrite <= m.written {
		return "Done!"
	}
	return "Writing songs...\n" + m.progress.View() + fmt.Sprintf("\n%d/%d", m.written, m.toWrite)
}

func writeSongMd(songs map[int]*song) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	for _, song := range songs {
		song := song
		song.lock.RLock()
		defer song.lock.RUnlock()

		title := cases.Title(language.AmericanEnglish).String(
			strings.ReplaceAll(
				song.Attrs["Title"],
				"/", "-"),
		)

		if title == "" {
			return ErrorModel{error: fmt.Errorf("song %d has no title", song.ID)}, nil
		}

		dir := filepath.Join("songs", title)

		cmds = append(cmds, func() tea.Msg {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return errMsg{err}
			}

			data, err := json.MarshalIndent(song, "", "  ")
			if err != nil {
				return errMsg{err}
			}
			if err := os.WriteFile(filepath.Join(dir, title+".json"), data, 0644); err != nil {
				return errMsg{err}
			}

			data = []byte(strings.TrimSpace(song.Attrs["Lyrics"]) + "\n")
			if len(data) > 1 {
				os.WriteFile(filepath.Join(dir, title+".lyrics.md"), data, 0644)
			}
			data = []byte(strings.TrimSpace(song.Attrs["Tab"]) + "\n")
			if len(data) > 1 {
				os.WriteFile(filepath.Join(dir, title+".tab.md"), data, 0644)
			}

			return songMdWrittenMsg{}
		}, func() tea.Msg {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return errMsg{err}
			}

			tab := strings.TrimSpace(song.Attrs["Tab"]) + "\n"
			if len(tab) <= 1 {
				return songMdWrittenMsg{}
			}

			pat := regexp.MustCompile(`\(([A-G].*?)\)`)
			tab = pat.ReplaceAllString(tab, `[$1]`)

			tab = regexp.MustCompile(`\[([^A-G].+?)\]`).ReplaceAllString(tab, `[*$1]`)

			tab = fmt.Sprintf(`{title: %s}
{artist: John Stewart}
{composer: %s}
{album: %s}

`, title, song.Attrs["Songwriter"], song.Attrs["Recording"]) + tab

			os.WriteFile(filepath.Join(dir, title+".chordpro"), []byte(tab), 0644)

			return songMdWrittenMsg{}
		})
	}

	return writeSongMdModel{
		progress: progress.NewModel(),
		toWrite:  len(songs) * 2,
	}, tea.Batch(cmds...)
}

type songMdWrittenMsg struct{}
