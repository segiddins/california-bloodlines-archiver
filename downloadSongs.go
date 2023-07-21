package main

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/anaskhan96/soup"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/exp/maps"
)

type downloadSongsModel struct {
	songs map[int]*song

	debug []string

	downloadedCount int
}

// Init implements tea.Model.
func (downloadSongsModel) Init() tea.Cmd {
	panic("unimplemented")
}

// Update implements tea.Model.
func (m downloadSongsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tabDownloadedMsg:
		m.downloadedCount++

		song := m.songs[msg.id]
		if song.Attrs == nil {
			song.Attrs = map[string]string{}
		}
		for k, v := range msg.attrs {
			if e, ok := song.Attrs[k]; ok && e != v {
				return ErrorModel{error: fmt.Errorf("duplicate key for song %d %q: %q and %q", song.ID, k, e, v)}, nil
			}
			song.Attrs[k] = v
		}
	case lyricsDownloadedMsg:
		m.downloadedCount++

		song := m.songs[msg.id]
		if song.Attrs == nil {
			song.Attrs = map[string]string{}
		}
		for k, v := range msg.attrs {
			if e, ok := song.Attrs[k]; ok && e != v {
				return ErrorModel{error: fmt.Errorf("duplicate key for song %d %q: %q and %q", song.ID, k, e, v)}, nil
			}
			song.Attrs[k] = v
		}
	case songsWrittenMsg:
		return writeSongMd(m.songs)
	case errMsg:
		return ErrorModel{error: msg.error}, nil
	default:
		_ = msg
	}

	if m.downloadedCount == 2*len(m.songs) {
		return m, writeSongs(maps.Values(m.songs))
	}

	return m, nil
}

// View implements tea.Model.
func (m downloadSongsModel) View() string {
	if m.downloadedCount == 2*len(m.songs) {
		return "All songs downloaded!"
	}

	return "Downloading songs ..." + strings.Join(m.debug, "\n")
}

var _ tea.Model = downloadSongsModel{}

func downloadSongs(songs map[int]*song) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	for _, song := range songs {
		song := song
		cmds = append(cmds,
			doDownload(song.ID, "http://californiabloodlines.com/displaytab.lasso?-KeyValue=%d&-Token.Action=", func(id int, attrs map[string]string) tea.Msg {
				return tabDownloadedMsg{id, attrs}
			}),
			doDownload(song.ID, "http://californiabloodlines.com/display.lasso?-KeyValue=%d&-Token.Action=", func(id int, attrs map[string]string) tea.Msg {
				return lyricsDownloadedMsg{id, attrs}
			}),
		)
	}

	return downloadSongsModel{
		songs: songs,
	}, tea.Batch(cmds...)
}

type tabDownloadedMsg struct {
	id    int
	attrs map[string]string
}

type lyricsDownloadedMsg struct {
	id    int
	attrs map[string]string
}

func doDownload(id int, format string, onDone func(id int, attrs map[string]string) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		u, err := url.Parse(fmt.Sprintf(format, id))
		if err != nil {
			return errMsg{err}
		}

		resp, err := httpClient.Get(u.String())
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg{err}
		}
		root := soup.HTMLParse(string(body))
		if root.Error != nil {
			return errMsg{root.Error}
		}

		attrs := map[string]string{}

		rows := root.FindAll("tr")
		for _, row := range rows {
			l := row.Find("td", "align", "left")
			if l.Error != nil {
				continue
			}

			r := l.FindNextElementSibling()
			if r.Error != nil || r.Pointer.Data != "td" {
				continue
			}

			attrs[strings.TrimRight(l.FullText(), ":")] = r.FullText()
		}

		return onDone(id, attrs)
	}
}
