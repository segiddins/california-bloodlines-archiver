package main

import (
	"fmt"
	"io"
	netURL "net/url"

	"github.com/anaskhan96/soup"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type searchCriteriaMsg struct {
	criteria map[string][]string
	postURL  *netURL.URL
}

type getSearchCriteriaModel struct {
	spinner spinner.Model
}

// Init implements tea.Model.
func (m getSearchCriteriaModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		getSearchOptions,
	)
}

// Update implements tea.Model.
func (m getSearchCriteriaModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case errMsg:
		return ErrorModel{msg}, nil
	case searchCriteriaMsg:
		return doSearches(msg.criteria, msg.postURL)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View implements tea.Model.
func (m getSearchCriteriaModel) View() string {
	return fmt.Sprintf("%s fetching search criteria ...", m.spinner.View())
}

var _ tea.Model = getSearchCriteriaModel{}

func getSearchOptions() tea.Msg {
	url, err := netURL.Parse("http://californiabloodlines.com/search.lasso")
	if err != nil {
		return errMsg{err}
	}

	resp, err := httpClient.Get(url.String())
	if err != nil {
		return errMsg{err}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return errMsg{err}
	}
	doc := soup.HTMLParse(string(body))
	if doc.Error != nil {
		return errMsg{doc.Error}
	}

	form := doc.FindStrict("form", "action", "listing.lasso")
	if form.Error != nil {
		return errMsg{form.Error}
	}
	postURL, err := url.Parse(form.Attrs()["action"])
	if err != nil {
		return errMsg{err}
	}
	searchCriteriaMsg := searchCriteriaMsg{
		criteria: map[string][]string{},
		postURL:  postURL,
	}
	for _, sel := range form.FindAll("select") {
		if sel.Error != nil {
			return errMsg{sel.Error}
		}
		name := sel.Attrs()["name"]

		for _, opt := range sel.FindAll("option") {
			if opt.Error != nil {
				return errMsg{opt.Error}
			}
			value := opt.Attrs()["value"]
			if value == "" {
				continue
			}
			searchCriteriaMsg.criteria[name] = append(searchCriteriaMsg.criteria[name], value)
		}
	}

	return searchCriteriaMsg
}
