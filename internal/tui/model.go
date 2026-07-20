package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	cookiemodel "cookiex/internal/cookie"
	exporter "cookiex/internal/export"
	"cookiex/internal/history"
	requestmodel "cookiex/internal/request"
	"cookiex/internal/vault"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type focusArea int

const (
	focusProfile focusArea = iota
	focusMethod
	focusURL
	focusHeaders
	focusBody
	focusResult
)

type resultTab int

const (
	tabResponse resultTab = iota
	tabRequest
	tabCode
)

type ProfileStore interface {
	Save(profile vault.Profile) error
	Load(name string) (vault.Profile, error)
	List() ([]string, error)
}

type RequestRunner interface {
	Send(context.Context, requestmodel.Spec, []cookiemodel.Cookie) (requestmodel.Response, error)
}

// ProfileSyncer refreshes a profile's cookies from the original Chrome profile.
type ProfileSyncer interface {
	Sync(ctx context.Context, profile vault.Profile) (vault.Profile, error)
}

// HistoryStore persists playground request history and named presets.
type HistoryStore interface {
	AppendHistory(entry history.Entry) error
	ListHistory() []history.Entry
	SavePreset(name string, entry history.Entry) error
	ListPresets() []history.Entry
	LoadPreset(name string) (history.Entry, error)
}

type Options struct {
	Profiles    ProfileStore
	Runner      RequestRunner
	Syncer      ProfileSyncer
	History     HistoryStore
	ProfileName string
	URL         string
	Method      string
}

type Model struct {
	profiles ProfileStore
	runner   RequestRunner
	syncer   ProfileSyncer
	history  HistoryStore

	profileNames []string
	profileIdx   int
	profile      vault.Profile

	methods   []string
	methodIdx int

	urlInput textinput.Model
	body     textarea.Model
	viewport viewport.Model

	headers      []HeaderRow
	headerIdx    int
	editingHdr   bool
	editExisting bool
	hdrName      textinput.Model
	hdrValue     textinput.Model
	hdrFocusVal  bool

	savingPreset bool
	presetName   textinput.Model
	historyIdx   int
	presetIdx    int

	focus      focusArea
	resultTab  resultTab
	codeFormat int

	width  int
	height int

	sending bool
	syncing bool
	status  string
	err     string

	lastSpec requestmodel.Spec
	lastResp *requestmodel.Response
	cancel   context.CancelFunc
}

type sendDoneMsg struct {
	spec requestmodel.Spec
	resp requestmodel.Response
	err  error
}

type syncDoneMsg struct {
	profile vault.Profile
	oldCount int
	err      error
}

func New(opts Options) (*Model, error) {
	names, err := opts.Profiles.List()
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no cookie profiles found; run cookiex import first")
	}

	profileIdx := 0
	if opts.ProfileName != "" {
		found := false
		for i, name := range names {
			if name == opts.ProfileName {
				profileIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("profile %q not found", opts.ProfileName)
		}
	}

	profile, err := opts.Profiles.Load(names[profileIdx])
	if err != nil {
		return nil, err
	}

	urlInput := textinput.New()
	urlInput.Placeholder = "https://api.example.com/path"
	urlInput.CharLimit = 2048
	urlInput.Width = 80
	if opts.URL != "" {
		urlInput.SetValue(opts.URL)
	} else {
		urlInput.SetValue("https://" + profile.Host + "/")
	}

	body := textarea.New()
	body.Placeholder = "optional request body"
	body.SetHeight(4)
	body.SetWidth(80)
	body.ShowLineNumbers = false

	hdrName := textinput.New()
	hdrName.Placeholder = "Header-Name"
	hdrName.CharLimit = 128
	hdrName.Width = 24
	hdrValue := textinput.New()
	hdrValue.Placeholder = "value"
	hdrValue.CharLimit = 2048
	hdrValue.Width = 40

	presetName := textinput.New()
	presetName.Placeholder = "preset-name"
	presetName.CharLimit = 64
	presetName.Width = 32

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	methodIdx := 0
	method := strings.ToUpper(strings.TrimSpace(opts.Method))
	if method == "" {
		method = http.MethodGet
	}
	for i, m := range methods {
		if m == method {
			methodIdx = i
			break
		}
	}

	m := &Model{
		profiles:     opts.Profiles,
		runner:       opts.Runner,
		syncer:       opts.Syncer,
		history:      opts.History,
		profileNames: names,
		profileIdx:   profileIdx,
		profile:      profile,
		methods:      methods,
		methodIdx:    methodIdx,
		urlInput:     urlInput,
		body:         body,
		viewport:     viewport.New(80, 12),
		headers:      EnsureHostDerivedHeaders(urlInput.Value(), HeadersFromProfile(profile)),
		hdrName:      hdrName,
		hdrValue:     hdrValue,
		presetName:   presetName,
		historyIdx:   -1,
		presetIdx:    -1,
		focus:        focusURL,
		status:       "Enter send · Ctrl+R sync · Ctrl+H history · Ctrl+P/O presets · Ctrl+S save · q quit",
	}
	m.urlInput.Focus()
	m.refreshViewport()
	return m, nil
}

func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.refreshViewport()
		return m, nil

	case sendDoneMsg:
		m.sending = false
		m.cancel = nil
		if msg.err != nil {
			m.err = msg.err.Error()
			m.status = "request failed"
			m.lastResp = nil
		} else {
			m.err = ""
			m.lastSpec = msg.spec
			resp := msg.resp
			m.lastResp = &resp
			matched := 0
			if parsed, err := requestmodel.ParseURL(msg.spec.URL); err == nil {
				matched = len(requestmodel.MatchedCookieNames(parsed, m.profile.Cookies, time.Now()))
			}
			m.status = fmt.Sprintf("%s · %s · %d cookies", resp.Status, resp.Duration.Round(time.Millisecond), matched)
			m.resultTab = tabResponse
			m.recordHistory(msg.spec)
		}
		m.refreshViewport()
		return m, nil

	case syncDoneMsg:
		m.syncing = false
		if msg.err != nil {
			m.err = msg.err.Error()
			m.status = "sync failed"
		} else {
			m.err = ""
			m.profile = msg.profile
			m.headers = HeadersFromProfile(msg.profile)
			m.headerIdx = 0
			m.status = fmt.Sprintf("synced %d → %d cookies", msg.oldCount, len(msg.profile.Cookies))
			m.refreshViewport()
		}
		return m, nil

	case tea.KeyMsg:
		if m.savingPreset {
			return m.updatePresetName(msg)
		}
		if m.editingHdr {
			return m.updateHeaderEditor(msg)
		}
		if m.focus == focusURL {
			if msg.String() == "tab" || msg.String() == "shift+tab" || msg.String() == "enter" ||
				msg.String() == "ctrl+enter" || msg.String() == "ctrl+j" || msg.String() == "ctrl+s" ||
				msg.String() == "ctrl+r" || msg.String() == "ctrl+h" || msg.String() == "ctrl+p" ||
				msg.String() == "ctrl+o" || msg.String() == "q" || msg.Type == tea.KeyCtrlC {
				// fall through to global keys after blur handling below
			} else {
				var cmd tea.Cmd
				m.urlInput, cmd = m.urlInput.Update(msg)
				return m, cmd
			}
		}
		if m.focus == focusBody {
			switch msg.String() {
			case "tab", "shift+tab", "ctrl+enter", "ctrl+j", "ctrl+s", "ctrl+r", "ctrl+h", "ctrl+p", "ctrl+o", "q":
			default:
				if msg.Type != tea.KeyCtrlC {
					var cmd tea.Cmd
					m.body, cmd = m.body.Update(msg)
					return m, cmd
				}
			}
		}
		if m.focus == focusResult {
			switch msg.String() {
			case "tab", "shift+tab", "enter", "ctrl+enter", "ctrl+j", "ctrl+s", "ctrl+r", "ctrl+h", "ctrl+p", "ctrl+o", "q", "1", "2", "3", "[", "]":
			default:
				if msg.Type != tea.KeyCtrlC {
					var cmd tea.Cmd
					m.viewport, cmd = m.viewport.Update(msg)
					return m, cmd
				}
			}
		}

		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "q":
			if m.focus != focusURL && m.focus != focusBody {
				return m, tea.Quit
			}
		case "tab":
			m.cycleFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil
		case "enter", "ctrl+enter", "ctrl+j":
			return m, m.send()
		case "ctrl+r":
			return m, m.syncProfile()
		case "ctrl+h":
			m.cycleHistory(1)
			return m, nil
		case "ctrl+p":
			m.startSavePreset()
			return m, nil
		case "ctrl+o":
			m.cyclePreset(1)
			return m, nil
		case "ctrl+s":
			return m, m.saveHeaders()
		case "left", "h":
			if m.focus == focusProfile {
				m.changeProfile(-1)
				return m, nil
			}
			if m.focus == focusMethod {
				m.methodIdx = (m.methodIdx - 1 + len(m.methods)) % len(m.methods)
				return m, nil
			}
			if m.focus == focusResult {
				m.cycleTab(-1)
				m.refreshViewport()
				return m, nil
			}
		case "right", "l":
			if m.focus == focusProfile {
				m.changeProfile(1)
				return m, nil
			}
			if m.focus == focusMethod {
				m.methodIdx = (m.methodIdx + 1) % len(m.methods)
				return m, nil
			}
			if m.focus == focusResult {
				m.cycleTab(1)
				m.refreshViewport()
				return m, nil
			}
		case "1":
			if m.focus == focusResult {
				m.resultTab = tabResponse
				m.refreshViewport()
				return m, nil
			}
		case "2":
			if m.focus == focusResult {
				m.resultTab = tabRequest
				m.refreshViewport()
				return m, nil
			}
		case "3":
			if m.focus == focusResult {
				m.resultTab = tabCode
				m.refreshViewport()
				return m, nil
			}
		case "[":
			if m.focus == focusResult && m.resultTab == tabCode {
				m.codeFormat = (m.codeFormat - 1 + len(exporter.SupportedFormats)) % len(exporter.SupportedFormats)
				m.refreshViewport()
				return m, nil
			}
		case "]":
			if m.focus == focusResult && m.resultTab == tabCode {
				m.codeFormat = (m.codeFormat + 1) % len(exporter.SupportedFormats)
				m.refreshViewport()
				return m, nil
			}
		case "up", "k":
			if m.focus == focusHeaders && len(m.headers) > 0 {
				m.headerIdx = (m.headerIdx - 1 + len(m.headers)) % len(m.headers)
				return m, nil
			}
		case "down", "j":
			if m.focus == focusHeaders && len(m.headers) > 0 {
				m.headerIdx = (m.headerIdx + 1) % len(m.headers)
				return m, nil
			}
		case " ":
			if m.focus == focusHeaders && len(m.headers) > 0 {
				m.headers[m.headerIdx].Enabled = !m.headers[m.headerIdx].Enabled
				return m, nil
			}
		case "p":
			if m.focus == focusHeaders && len(m.headers) > 0 {
				m.headers[m.headerIdx].FromProfile = !m.headers[m.headerIdx].FromProfile
				return m, nil
			}
		case "a":
			if m.focus == focusHeaders {
				m.startAddHeader()
				return m, nil
			}
		case "e":
			if m.focus == focusHeaders && len(m.headers) > 0 {
				m.startEditHeader(m.headerIdx)
				return m, nil
			}
		case "d", "x":
			if m.focus == focusHeaders && len(m.headers) > 0 {
				m.headers = append(m.headers[:m.headerIdx], m.headers[m.headerIdx+1:]...)
				if m.headerIdx >= len(m.headers) && m.headerIdx > 0 {
					m.headerIdx--
				}
				return m, nil
			}
		}
	}

	return m, nil
}

func (m *Model) updateHeaderEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editingHdr = false
		m.hdrName.Blur()
		m.hdrValue.Blur()
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.hdrName.Value())
		value := m.hdrValue.Value()
		if name != "" {
			if m.editExisting && m.headerIdx >= 0 && m.headerIdx < len(m.headers) {
				m.headers[m.headerIdx].Name = name
				m.headers[m.headerIdx].Value = value
			} else {
				m.headers = append(m.headers, HeaderRow{
					Name: name, Value: value, Enabled: true, FromProfile: false,
				})
				m.headerIdx = len(m.headers) - 1
			}
		}
		m.editingHdr = false
		m.editExisting = false
		m.hdrName.Blur()
		m.hdrValue.Blur()
		return m, nil
	case "tab":
		m.hdrFocusVal = !m.hdrFocusVal
		if m.hdrFocusVal {
			m.hdrName.Blur()
			m.hdrValue.Focus()
		} else {
			m.hdrValue.Blur()
			m.hdrName.Focus()
		}
		return m, nil
	}
	var cmd tea.Cmd
	if m.hdrFocusVal {
		m.hdrValue, cmd = m.hdrValue.Update(msg)
	} else {
		m.hdrName, cmd = m.hdrName.Update(msg)
	}
	return m, cmd
}

func (m *Model) startAddHeader() {
	m.editingHdr = true
	m.editExisting = false
	m.hdrFocusVal = false
	m.hdrName.SetValue("")
	m.hdrValue.SetValue("")
	m.hdrName.Placeholder = "Header-Name"
	m.hdrName.Focus()
	m.hdrValue.Blur()
}

func (m *Model) startEditHeader(idx int) {
	m.editingHdr = true
	m.editExisting = true
	m.headerIdx = idx
	m.hdrFocusVal = false
	m.hdrName.SetValue(m.headers[idx].Name)
	m.hdrValue.SetValue(m.headers[idx].Value)
	m.hdrName.Placeholder = "Header-Name"
	m.hdrName.Focus()
	m.hdrValue.Blur()
}

func (m *Model) startSavePreset() {
	if m.history == nil {
		m.err = "history is not available"
		return
	}
	m.savingPreset = true
	m.presetName.SetValue("")
	m.presetName.Focus()
	m.urlInput.Blur()
	m.body.Blur()
}

func (m *Model) updatePresetName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.savingPreset = false
		m.presetName.Blur()
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.presetName.Value())
		m.savingPreset = false
		m.presetName.Blur()
		if name == "" {
			m.err = "preset name is required"
			return m, nil
		}
		entry := HistoryEntryFromForm(m.profile.Name, m.methods[m.methodIdx], m.urlInput.Value(), m.body.Value(), m.headers)
		if err := m.history.SavePreset(name, entry); err != nil {
			m.err = err.Error()
			m.status = "preset save failed"
			return m, nil
		}
		m.err = ""
		m.status = fmt.Sprintf("saved preset %s", name)
		return m, nil
	}
	var cmd tea.Cmd
	m.presetName, cmd = m.presetName.Update(msg)
	return m, cmd
}

func (m *Model) recordHistory(spec requestmodel.Spec) {
	if m.history == nil {
		return
	}
	entry := HistoryEntryFromForm(m.profile.Name, spec.Method, spec.URL, spec.Body, m.headers)
	if err := m.history.AppendHistory(entry); err != nil {
		m.err = err.Error()
	}
}

func (m *Model) cycleHistory(delta int) {
	if m.history == nil {
		m.err = "history is not available"
		return
	}
	items := m.history.ListHistory()
	if len(items) == 0 {
		m.status = "no history yet"
		return
	}
	if m.historyIdx < 0 {
		m.historyIdx = 0
	} else {
		m.historyIdx = (m.historyIdx + delta + len(items)) % len(items)
	}
	m.applyEntry(items[m.historyIdx])
	m.status = fmt.Sprintf("history %d/%d", m.historyIdx+1, len(items))
	m.err = ""
}

func (m *Model) cyclePreset(delta int) {
	if m.history == nil {
		m.err = "history is not available"
		return
	}
	items := m.history.ListPresets()
	if len(items) == 0 {
		m.status = "no presets yet"
		return
	}
	if m.presetIdx < 0 {
		m.presetIdx = 0
	} else {
		m.presetIdx = (m.presetIdx + delta + len(items)) % len(items)
	}
	m.applyEntry(items[m.presetIdx])
	m.status = fmt.Sprintf("preset %s (%d/%d)", items[m.presetIdx].Name, m.presetIdx+1, len(items))
	m.err = ""
}

func (m *Model) applyEntry(entry history.Entry) {
	if entry.Profile != "" {
		for i, name := range m.profileNames {
			if name == entry.Profile {
				if i != m.profileIdx {
					m.profileIdx = i
					if profile, err := m.profiles.Load(name); err == nil {
						m.profile = profile
					}
				}
				break
			}
		}
	}
	method := strings.ToUpper(strings.TrimSpace(entry.Method))
	for i, candidate := range m.methods {
		if candidate == method {
			m.methodIdx = i
			break
		}
	}
	m.urlInput.SetValue(entry.URL)
	m.body.SetValue(entry.Body)
	if len(entry.Headers) > 0 {
		m.headers = EnsureHostDerivedHeaders(entry.URL, ApplyHistoryHeaders(entry.Headers))
	} else {
		m.headers = EnsureHostDerivedHeaders(entry.URL, HeadersFromProfile(m.profile))
	}
	m.headerIdx = 0
}

func (m *Model) View() string {
	title := titleStyle.Render("Cookiex Playground")
	profile := fmt.Sprintf("Profile: %s", m.profileNames[m.profileIdx])
	method := fmt.Sprintf("Method: %s", m.methods[m.methodIdx])
	if m.focus == focusProfile {
		profile = focusedStyle.Render(profile + " ◂▸")
	} else {
		profile = blurredStyle.Render(profile)
	}
	if m.focus == focusMethod {
		method = focusedStyle.Render(method + " ◂▸")
	} else {
		method = blurredStyle.Render(method)
	}

	urlLabel := "URL"
	if m.focus == focusURL {
		urlLabel = focusedStyle.Render("URL")
	}
	headerBox := m.renderHeaders()
	bodyLabel := "Body"
	if m.focus == focusBody {
		bodyLabel = focusedStyle.Render("Body")
	}

	tabs := m.renderTabs()
	result := m.viewport.View()
	status := statusStyle.Render(m.status)
	if m.err != "" {
		status = errorStyle.Render(m.err)
	}
	if m.sending {
		status = statusStyle.Render("Sending…")
	}
	if m.syncing {
		status = statusStyle.Render("Syncing…")
	}

	help := helpStyle.Render("Tab · ←/→ · Space · a/e/p/d headers · Enter send · Ctrl+R sync · Ctrl+H history · Ctrl+P save preset · Ctrl+O open preset · Ctrl+S save · q quit")

	if m.savingPreset {
		help = helpStyle.Render("Preset name · Enter save · Esc cancel")
		return lipgloss.JoinVertical(lipgloss.Left,
			title,
			lipgloss.JoinHorizontal(lipgloss.Top, profile, "   ", method),
			urlLabel,
			m.urlInput.View(),
			headerBox,
			bodyLabel,
			m.body.View(),
			focusedStyle.Render("Save preset: ")+m.presetName.View(),
			tabs,
			result,
			status,
			help,
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		lipgloss.JoinHorizontal(lipgloss.Top, profile, "   ", method),
		urlLabel,
		m.urlInput.View(),
		headerBox,
		bodyLabel,
		m.body.View(),
		tabs,
		result,
		status,
		help,
	)
}

func (m *Model) renderHeaders() string {
	var b strings.Builder
	label := "Headers"
	if m.focus == focusHeaders {
		label = focusedStyle.Render("Headers")
	}
	b.WriteString(label)
	b.WriteString("\n")
	if m.editingHdr {
		b.WriteString(fmt.Sprintf("  edit: %s  %s  (Enter save · Esc cancel · Tab field)\n", m.hdrName.View(), m.hdrValue.View()))
		return panelStyle.Render(b.String())
	}
	if len(m.headers) == 0 {
		b.WriteString("  (none — press a to add)\n")
	}
	for i, row := range m.headers {
		mark := " "
		if row.Enabled {
			mark = "✓"
		}
		src := "[R]"
		if row.FromProfile {
			src = "[P]"
		}
		line := fmt.Sprintf("  %s %-20s %-40s %s", mark, row.Name, truncate(row.Value, 40), src)
		if m.focus == focusHeaders && i == m.headerIdx {
			line = focusedStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return panelStyle.Render(b.String())
}

func (m *Model) renderTabs() string {
	names := []string{"Response", "Request", "Code"}
	parts := make([]string, 0, 3)
	for i, name := range names {
		style := tabStyle
		if resultTab(i) == m.resultTab {
			style = activeTabStyle
		}
		if m.focus == focusResult && resultTab(i) == m.resultTab {
			style = focusedTabStyle
		}
		parts = append(parts, style.Render(fmt.Sprintf(" %s ", name)))
	}
	extra := ""
	if m.resultTab == tabCode {
		extra = "  " + blurredStyle.Render(fmt.Sprintf("[%s]  [/] cycle format", exporter.FormatLabel(exporter.SupportedFormats[m.codeFormat])))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...) + extra
}

func (m *Model) refreshViewport() {
	m.viewport.SetContent(m.resultContent())
	m.viewport.GotoTop()
}

func (m *Model) resultContent() string {
	switch m.resultTab {
	case tabRequest:
		return m.requestContent()
	case tabCode:
		return m.codeContent()
	default:
		return m.responseContent()
	}
}

func (m *Model) responseContent() string {
	if m.lastResp == nil {
		return "Send a request to see the response."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", m.lastResp.Status, m.lastResp.Duration.Round(time.Millisecond))
	names := make([]string, 0, len(m.lastResp.Headers))
	for name := range m.lastResp.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range m.lastResp.Headers.Values(name) {
			fmt.Fprintf(&b, "%s: %s\n", name, value)
		}
	}
	b.WriteByte('\n')
	body := m.lastResp.Body
	if json.Valid(body) {
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, body, "", "  "); err == nil {
			body = formatted.Bytes()
		}
	}
	b.Write(body)
	if m.lastResp.Truncated {
		b.WriteString("\n[response body truncated]")
	}
	return b.String()
}

func (m *Model) requestContent() string {
	spec := m.lastSpec
	if spec.URL == "" {
		spec = BuildSpec(m.methods[m.methodIdx], m.urlInput.Value(), m.body.Value(), m.headers)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", strings.ToUpper(orDefault(spec.Method, http.MethodGet)), spec.URL)
	fmt.Fprintf(&b, "Profile: %s (%s)\n\n", m.profile.Name, m.profile.Host)
	for _, name := range hdrsSorted(spec.Headers) {
		value := spec.Headers[name]
		if strings.EqualFold(name, "Cookie") {
			value = "[redacted]"
		}
		fmt.Fprintf(&b, "%s: %s\n", name, value)
	}
	b.WriteString(m.matchedCookiesLine(spec.URL))
	b.WriteByte('\n')
	if spec.Body != "" {
		b.WriteString("\n")
		b.WriteString(spec.Body)
	}
	return b.String()
}

func (m *Model) matchedCookiesLine(rawURL string) string {
	parsed, err := requestmodel.ParseURL(rawURL)
	if err != nil {
		return requestmodel.FormatMatchedCookiesLine(nil)
	}
	names := requestmodel.MatchedCookieNames(parsed, m.profile.Cookies, time.Now())
	return requestmodel.FormatMatchedCookiesLine(names)
}

func (m *Model) codeContent() string {
	spec := m.lastSpec
	if spec.URL == "" {
		spec = BuildSpec(m.methods[m.methodIdx], m.urlInput.Value(), m.body.Value(), m.headers)
	}
	format := exporter.SupportedFormats[m.codeFormat]
	code, err := exporter.Render(format, spec, m.profile.Cookies)
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("// %s — contains live credentials\n\n%s", exporter.FormatLabel(format), code)
}

func hdrsSorted(headers map[string]string) []string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})
	return names
}

func (m *Model) cycleFocus(delta int) {
	areas := []focusArea{focusProfile, focusMethod, focusURL, focusHeaders, focusBody, focusResult}
	idx := 0
	for i, a := range areas {
		if a == m.focus {
			idx = i
			break
		}
	}
	if m.focus == focusURL {
		m.headers = EnsureHostDerivedHeaders(m.urlInput.Value(), m.headers)
	}
	idx = (idx + delta + len(areas)) % len(areas)
	m.focus = areas[idx]
	m.urlInput.Blur()
	m.body.Blur()
	switch m.focus {
	case focusURL:
		m.urlInput.Focus()
	case focusBody:
		m.body.Focus()
	}
}

func (m *Model) cycleTab(delta int) {
	m.resultTab = resultTab((int(m.resultTab) + delta + 3) % 3)
}

func (m *Model) changeProfile(delta int) {
	m.profileIdx = (m.profileIdx + delta + len(m.profileNames)) % len(m.profileNames)
	profile, err := m.profiles.Load(m.profileNames[m.profileIdx])
	if err != nil {
		m.err = err.Error()
		return
	}
	m.profile = profile
	m.headers = EnsureHostDerivedHeaders(m.urlInput.Value(), HeadersFromProfile(profile))
	m.headerIdx = 0
	m.err = ""
	m.status = fmt.Sprintf("loaded profile %s", profile.Name)
}

func (m *Model) send() tea.Cmd {
	if m.sending || m.syncing {
		return nil
	}
	m.headers = EnsureHostDerivedHeaders(m.urlInput.Value(), m.headers)
	spec := BuildSpec(m.methods[m.methodIdx], m.urlInput.Value(), m.body.Value(), m.headers)
	if spec.URL == "" {
		m.err = "URL is required"
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	m.cancel = cancel
	m.sending = true
	m.err = ""
	m.status = "Sending…"
	cookies := m.profile.Cookies
	runner := m.runner
	return func() tea.Msg {
		resp, err := runner.Send(ctx, spec, cookies)
		cancel()
		return sendDoneMsg{spec: spec, resp: resp, err: err}
	}
}

func (m *Model) syncProfile() tea.Cmd {
	if m.syncing || m.sending {
		return nil
	}
	if m.syncer == nil {
		m.err = "sync is not available"
		return nil
	}
	m.syncing = true
	m.err = ""
	m.status = "Syncing…"
	syncer := m.syncer
	profile := m.profile
	oldCount := len(profile.Cookies)
	return func() tea.Msg {
		updated, err := syncer.Sync(context.Background(), profile)
		return syncDoneMsg{profile: updated, oldCount: oldCount, err: err}
	}
}

func (m *Model) saveHeaders() tea.Cmd {
	m.profile.Headers = ProfileHeadersFromRows(m.headers)
	if err := m.profiles.Save(m.profile); err != nil {
		m.err = err.Error()
		m.status = "save failed"
		return nil
	}
	m.err = ""
	m.status = fmt.Sprintf("saved %d default headers to profile %s", len(m.profile.Headers), m.profile.Name)
	return nil
}

func (m *Model) layout() {
	width := m.width
	if width < 40 {
		width = 40
	}
	m.urlInput.Width = width - 4
	m.body.SetWidth(width - 4)
	resultHeight := m.height - 22
	if resultHeight < 6 {
		resultHeight = 6
	}
	m.viewport.Width = width - 2
	m.viewport.Height = resultHeight
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	focusedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true)
	blurredStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	panelStyle      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	tabStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	activeTabStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true).Underline(true)
	focusedTabStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Underline(true)
	statusStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	helpStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)
