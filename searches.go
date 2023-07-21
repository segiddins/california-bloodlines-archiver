package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	netURL "net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/anaskhan96/soup"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/stopwatch"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/semaphore"
)

var (
	titleStyle        = lipgloss.NewStyle().MarginLeft(2)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	paginationStyle   = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	quitTextStyle     = lipgloss.NewStyle().Margin(1, 0, 2, 4)
)

type criteria struct {
	key, value string
}

type searchCompletedMsg struct {
	criteria criteria
	songs    []*song
	error
}

type searchStatus struct {
	searchCriteria criteria
	songs          []*song
	error

	duration time.Duration
}

type searchesModel struct {
	searches       map[criteria]*searchStatus
	sortedSearches []*searchStatus
	stopwatch      stopwatch.Model
	spinner        spinner.Model
	completedCount int

	sema *semaphore.Weighted

	tea.WindowSizeMsg
}

func (m *searchesModel) sortSearches() {
	slices.SortFunc(m.sortedSearches, func(a, b *searchStatus) bool {
		if a.error != nil && b.error == nil { // errors first
			return true
		}
		if a.error == nil && b.error != nil { // errors first
			return false
		}

		if a.songs == nil && b.songs != nil { // incomplete searches first
			return true
		} else if a.songs != nil && b.songs == nil { // incomplete searches first
			return false
		}

		if len(a.songs) != len(b.songs) {
			return len(a.songs) > len(b.songs)
		}

		return a.searchCriteria.value < b.searchCriteria.value
	})
}

// Init implements tea.Model.
func (m *searchesModel) Init() tea.Cmd {
	panic("unimplemented")
}

// Update implements tea.Model.
func (m *searchesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case stopwatch.StartStopMsg, stopwatch.TickMsg:
		var cmd tea.Cmd
		m.stopwatch, cmd = m.stopwatch.Update(msg)
		return m, cmd
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.WindowSizeMsg:
		m.WindowSizeMsg = msg
	case searchCompletedMsg:
		search := m.searches[msg.criteria]
		search.songs = msg.songs
		search.duration = m.stopwatch.Elapsed()
		search.error = msg.error
		m.completedCount++
		m.sortSearches()
		if m.completedCount == len(m.searches) {
			songsList := make([]*song, 0)
			for _, search := range m.sortedSearches {
				songsList = append(songsList, search.songs...)
			}
			return m, writeSongs(songsList)
		}
	case songsWrittenMsg:
		return downloadSongs(msg.songs)
	case errMsg:
		return ErrorModel{msg}, tea.Quit
	}
	return m, nil
}

func (m *searchesModel) searchItem(s *searchStatus) string {
	width := m.Width - 4 - 2

	var left, right string
	if s.duration > 0 {
		right = s.duration.String()
	} else {
		right = m.stopwatch.View()
	}

	width -= lipgloss.Width(right)

	leftStyle := lipgloss.NewStyle().Align(lipgloss.Left).Width(width)

	label := fmt.Sprintf("%s: %s", s.searchCriteria.key, s.searchCriteria.value)
	if s.error != nil {
		left = fmt.Sprintf("❌ %s\n  %s", label, s.error.Error())
	} else if s.songs == nil {
		left = fmt.Sprintf("%s %s", m.spinner.View(), label)
	} else {
		left = fmt.Sprintf("✅ %s (%d songs)", label, len(s.songs))
	}

	left = leftStyle.Render(left)

	str := lipgloss.JoinHorizontal(lipgloss.Top,
		left,
		right,
	)

	return itemStyle.Width(m.Width).Render(str)
}

// View implements tea.Model.
func (m *searchesModel) View() string {
	if m.completedCount == len(m.searches) {
		errors := make([]string, 0)
		for _, search := range m.sortedSearches {
			if search.error == nil {
				break
			}

			errors = append(errors, fmt.Sprintf("%s: %s %s", search.searchCriteria.key, search.searchCriteria.value, search.error.Error()))
		}

		if len(errors) > 0 {
			return strings.Join(errors, "\n")
		}
		return "Done!"
	}

	lines := m.Height - 2

	s := lipgloss.NewStyle().Width(m.Width)
	header := s.Render(fmt.Sprintf("Doing searches (%d/%d)", m.completedCount, len(m.searches)))
	lines -= lipgloss.Height(header)

	items := make([]string, 0, len(m.searches))
	items = append(items, header)

	for _, search := range m.sortedSearches {
		items = append(items, s.Render(m.searchItem(search)))

		lines -= lipgloss.Height(items[len(items)-1])
		if lines < 0 {
			items = items[:len(items)-1]
			break
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		items...,
	)
}

func doSearches(searchCriteria map[string][]string, postURL *url.URL) (tea.Model, tea.Cmd) {
	searches := map[criteria]*searchStatus{}
	cmds := make([]tea.Cmd, 0)
	sema := semaphore.NewWeighted(10)

	for key, values := range searchCriteria {
		for _, value := range values {
			criteria := criteria{key, value}
			search := &searchStatus{
				searchCriteria: criteria,
			}
			searches[criteria] = search

			cmds = append(cmds, doSearch(criteria, postURL, sema))
		}
	}

	m := searchesModel{
		searches:       searches,
		sortedSearches: maps.Values(searches),
		stopwatch:      stopwatch.NewWithInterval(time.Second / 10),
		spinner:        spinner.NewModel(),
		sema:           sema,
	}
	m.sortSearches()

	cmds = append(cmds, m.stopwatch.Start(), m.spinner.Tick)

	return &m, tea.Batch(cmds...)
}

func doSearch(criteria criteria, postURL *url.URL, sema *semaphore.Weighted) tea.Cmd {
	inner := func() tea.Msg {
		if err := sema.Acquire(context.TODO(), 1); err != nil {
			return errMsg{err}
		}
		defer sema.Release(1)
		resp, err := httpClient.PostForm(postURL.String(), netURL.Values{
			criteria.key: []string{criteria.value},
		})
		if err != nil {
			return errMsg{err}
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errMsg{err}
		}
		doc := soup.HTMLParse(string(body))

		tables := doc.FindAll("table")
		if len(tables) == 0 {
			return errMsg{fmt.Errorf("no tables found")}
		}
		table := tables[len(tables)-1]
		if table.Error != nil {
			return errMsg{table.Error}
		}

		songs := map[int]*song{}

		for _, a := range table.FindAll("a") {
			if a.Error != nil {
				return errMsg{a.Error}
			}

			href := a.Attrs()["href"]
			if href == "" {
				continue
			}

			url, err := url.Parse(href)
			if err != nil {
				return errMsg{err}
			}
			ids := url.Query().Get("-KeyValue")
			if ids == "" {
				continue
			}
			id, err := strconv.Atoi(ids)
			if err != nil {
				return errMsg{err}
			}

			s := songs[id]
			if s == nil {
				s = &song{
					ID:      id,
					Critera: map[string][]string{},
				}
				songs[id] = s
			}

			s.Critera[criteria.key] = append(s.Critera[criteria.key], criteria.value)
		}

		return searchCompletedMsg{
			criteria: criteria,
			songs:    maps.Values(songs),
		}
	}

	return func() tea.Msg {
		v := inner()
		if e, ok := v.(errMsg); ok {
			return searchCompletedMsg{
				criteria: criteria,
				error:    e,
			}
		}
		return v
	}
}

func writeSongs(songsList []*song) tea.Cmd {

	return func() tea.Msg {
		songs := map[int]*song{}
		for _, song := range songsList {
			if s, ok := songs[song.ID]; ok && s != song {
				s.lock.Lock()
				song.lock.Lock()
				s.Critera = mergeCriteria(s.Critera, song.Critera)
				s.lock.Unlock()
				song.lock.Unlock()
			} else {
				songs[song.ID] = song
			}
		}

		for _, song := range songs {
			song.lock.Lock()
			for k, v := range song.Critera {
				slices.Sort(v)
				song.Critera[k] = slices.Compact(v)
			}
			song.lock.Unlock()
		}

		songSlice := maps.Values(songs)
		slices.SortFunc(songSlice, func(a, b *song) bool {
			return a.ID < b.ID
		})

		j, err := json.MarshalIndent(songSlice, "", "  ")
		if err != nil {
			return errMsg{error: err}
		}

		err = os.WriteFile("songs.json", j, 0644)
		if err != nil {
			return errMsg{err}
		}

		return songsWrittenMsg{
			songs: songs,
		}
	}
}

type songsWrittenMsg struct {
	songs map[int]*song
}

func mergeCriteria(a, b map[string][]string) map[string][]string {
	for key, values := range b {
		a[key] = append(a[key], values...)
	}
	return a
}
