package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	netURL "net/url"
	"os"
	"strconv"
	"sync"

	"github.com/anaskhan96/soup"
	"github.com/kr/pretty"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type MainModel struct {
	tea.Model
	width    int
	height   int
	quitting bool
}

func (m MainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd, windowCmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	m.Model, windowCmd = m.Model.Update(tea.WindowSizeMsg{Width: m.width,
		Height: m.height})
	return m, tea.Batch(cmd, windowCmd)
}

func (m MainModel) View() string {
	v := m.Model.View()
	if m.quitting {
		return v + "\nquitting..."
	} else {
		return v
	}
}

type errMsg struct{ error }

func (e errMsg) Error() string { return e.error.Error() }

type song struct {
	Title      string `json:"title,omitempty"`
	Songwriter string `json:"songwriter,omitempty"`
	Recording  string `json:"recording,omitempty"`
	Lyrics     string `json:"lyrics,omitempty"`
	Tab        string `json:"tab,omitempty"`

	Attrs   map[string]string
	Critera map[string][]string

	ID int `json:"id"`

	lock sync.RWMutex
}

type database struct {
	m sync.Mutex

	songs map[int]*song

	searchCriteria map[string][]string
}

type model struct {
	*database
	spinner  spinner.Model
	sub      chan struct{}
	quitting bool
	errors   []error
}

// A command that waits for the activity on a channel.
func waitForActivity(sub chan struct{}) tea.Cmd {
	return func() tea.Msg {
		<-sub
		return nil
	}
}

// Init implements tea.Model.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		getSearchOptions,       // start activity
		waitForActivity(m.sub), // wait for activity
	)
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyMsg:
		m.quitting = true
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case errMsg:
		m.errors = append(m.errors, msg.(errMsg))
		return m, waitForActivity(m.sub)
	case searchCriteriaMsg:
		m.searchCriteria = msg.(searchCriteriaMsg).criteria
		return m, waitForActivity(m.sub)
	case nil, tea.WindowSizeMsg:
		return m, nil
	default:
		m.errors = append(m.errors, fmt.Errorf("Unexpected message: %#v", msg))
		return m, tea.Quit
	}
}

// View implements tea.Model.
func (m model) View() string {
	return pretty.Sprintf("%# v\n\n%# v\n\n%s", m.database, m.errors, m.spinner.View())
}

func dosSearches(db *database, sub chan struct{}) tea.BatchMsg {

	searchWg := sync.WaitGroup{}

	for name, values := range db.searchCriteria {
		for _, value := range values {
			searchWg.Add(1)
			go func(name, value string) {
				defer searchWg.Done()

				postURL, err := url.Parse("")
				if err != nil {
					panic(err)
				}
				resp, err := http.PostForm(postURL.String(), netURL.Values{
					name: []string{value},
				})
				if err != nil {
					return
				}
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return
				}
				doc := soup.HTMLParse(string(body))

				tables := doc.FindAll("table")
				if len(tables) == 0 {
					panic("no tables found")
				}
				table := tables[len(tables)-1]
				if table.Error != nil {
					panic(table.Error)
				}

				for _, a := range table.FindAll("a") {
					if a.Error != nil {
						panic(a.Error)
					}

					href := a.Attrs()["href"]
					if href == "" {
						continue
					}

					url, err := url.Parse(href)
					if err != nil {
						panic(err)
					}
					ids := url.Query().Get("-KeyValue")
					if ids == "" {
						continue
					}
					id, err := strconv.Atoi(ids)
					if err != nil {
						panic(err)
					}

					db.m.Lock()
					defer db.m.Unlock()
					defer func() { sub <- struct{}{} }()

					s := db.songs[id]
					if s == nil {
						s = &song{
							ID:      id,
							Critera: map[string][]string{},
						}
						db.songs[id] = s
					}

					if s.Critera[name] == nil {
						s.Critera[name] = []string{}
					}
					s.Critera[name] = append(s.Critera[name], value)
				}
			}(name, value)
		}
	}

	searchWg.Wait()
	return nil
}

func main() {
	// p := tea.NewProgram(model{
	// 	sub:     make(chan struct{}),
	// 	spinner: spinner.New(),
	// 	database: &database{
	// 		songs:          map[int]*song{},
	// 		searchCriteria: map[string][]string{},
	// 	},
	// })

	p := tea.NewProgram(MainModel{
		Model: getSearchCriteriaModel{
			spinner: spinner.NewModel(),
		},
	})

	if _, err := p.Run(); err != nil {
		pretty.Println("could not start program:", err)
		os.Exit(1)
	}
}
