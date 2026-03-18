package main

import (
	_ "embed"
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/muesli/termenv"
	_ "modernc.org/sqlite"
	"gopkg.in/yaml.v3"
)

//go:embed ascii/avatar.txt
var avatarArt string

//go:embed ascii/name.txt
var nameArt string

//go:embed content.yaml
var contentYAML []byte

const (
	host = "0.0.0.0"
	port = "106"
)

// ── colors ────────────────────────────────────────────────────────────────────

var (
	colorPurple = lipgloss.Color("#C084FC")
	colorCyan   = lipgloss.Color("#67E8F9")
	colorGreen  = lipgloss.Color("#86EFAC")
	colorYellow = lipgloss.Color("#FDE68A")
	colorPink   = lipgloss.Color("#F9A8D4")
	colorOrange = lipgloss.Color("#FB923C")
	colorGray   = lipgloss.Color("#6B7280")
	colorWhite  = lipgloss.Color("#F9FAFB")
	colorDim    = lipgloss.Color("#9CA3AF")
)

// ── page ──────────────────────────────────────────────────────────────────────

type page int

const (
	pageHome page = iota
	pageAbout
	pageProjects
	pageContact
	pageGuestbook
	pageGuestbookWrite
)

// ── content ───────────────────────────────────────────────────────────────────

type content struct {
	TabAbout    string `yaml:"tab_about"`
	TabProjects string `yaml:"tab_projects"`
	TabContact  string `yaml:"tab_contact"`
	TabGuestbook string `yaml:"tab_guestbook"`

	Name   string   `yaml:"name"`
	Role   string   `yaml:"role"`
	City   string   `yaml:"city"`
	Status string   `yaml:"status"`
	Bio    []string `yaml:"bio"`

	SkillsTitle string   `yaml:"skills_title"`
	Skills      []string `yaml:"skills"`

	ProjectsTitle string    `yaml:"projects_title"`
	Projects      []project `yaml:"projects"`

	ContactTitle string    `yaml:"contact_title"`
	Contacts     []contact `yaml:"contacts"`

	GuestbookEmpty     string `yaml:"guestbook_empty"`
	GuestbookNameLabel string `yaml:"guestbook_name_label"`
	GuestbookMsgLabel  string `yaml:"guestbook_msg_label"`
	VisitorLabel       string `yaml:"visitor_label"`

	HelpNav    string `yaml:"help_nav"`
	HelpOpen   string `yaml:"help_open"`
	HelpBack   string `yaml:"help_back"`
	HelpLang   string `yaml:"help_lang"`
	HelpQuit   string `yaml:"help_quit"`
	HelpWrite  string `yaml:"help_write"`
	HelpSubmit string `yaml:"help_submit"`
}

type project struct {
	Name  string   `yaml:"name"`
	Desc  string   `yaml:"desc"`
	Links []string `yaml:"links"`
}

type contact struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
}

var en, zh content

func init() {
	var cf struct {
		En content `yaml:"en"`
		Zh content `yaml:"zh"`
	}
	if err := yaml.Unmarshal(contentYAML, &cf); err != nil {
		log.Fatalf("failed to parse content.yaml: %v", err)
	}
	en = cf.En
	zh = cf.Zh
}

// ── database ──────────────────────────────────────────────────────────────────

var db *sql.DB

type guestbookEntry struct {
	Name      string
	Message   string
	CreatedAt time.Time
}

func initDB(path string) error {
	if err := os.MkdirAll("./data", 0755); err != nil {
		return err
	}
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS visitors (
			ip TEXT PRIMARY KEY,
			first_seen DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS guestbook (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);
	`)
	return err
}

func registerVisitor(ip string) int {
	db.Exec(`INSERT OR IGNORE INTO visitors (ip) VALUES (?)`, ip)
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM visitors`).Scan(&count)
	return count
}

func fetchGuestbook() []guestbookEntry {
	rows, err := db.Query(`SELECT name, message, created_at FROM guestbook ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var entries []guestbookEntry
	for rows.Next() {
		var e guestbookEntry
		var ts int64
		if err := rows.Scan(&e.Name, &e.Message, &ts); err != nil {
			continue
		}
		e.CreatedAt = time.Unix(ts, 0)
		entries = append(entries, e)
	}
	return entries
}

func addGuestbookEntry(name, message string) {
	db.Exec(`INSERT INTO guestbook (name, message, created_at) VALUES (?, ?, ?)`, name, message, time.Now().Unix())
}

func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// ── model ─────────────────────────────────────────────────────────────────────

type model struct {
	page         page
	selected     int
	lang         string
	width        int
	height       int
	bioChars     int
	now          time.Time
	sparkleFrame int

	visitorNum   int
	guestEntries []guestbookEntry
	guestScroll  int

	nameInput   textinput.Model
	msgInput    textinput.Model
	activeInput int // 0=name 1=message
}

func initialModel(visitorNum int, entries []guestbookEntry) model {
	ni := textinput.New()
	ni.Placeholder = "your name"
	ni.CharLimit = 30
	ni.Width = 40
	ni.PromptStyle = lipgloss.NewStyle().Foreground(colorPurple)
	ni.TextStyle = lipgloss.NewStyle().Foreground(colorWhite)

	mi := textinput.New()
	mi.Placeholder = "leave a message..."
	mi.CharLimit = 100
	mi.Width = 40
	mi.PromptStyle = lipgloss.NewStyle().Foreground(colorPurple)
	mi.TextStyle = lipgloss.NewStyle().Foreground(colorWhite)

	return model{
		page:         pageHome,
		selected:     0,
		lang:         "en",
		now:          time.Now(),
		visitorNum:   visitorNum,
		guestEntries: entries,
		nameInput:    ni,
		msgInput:     mi,
	}
}

func (m model) c() content {
	if m.lang == "zh" {
		return zh
	}
	return en
}

// ── tick messages ─────────────────────────────────────────────────────────────

type typingMsg struct{}
type clockMsg time.Time
type sparkleMsg struct{}

func typingCmd() tea.Cmd {
	return tea.Tick(20*time.Millisecond, func(t time.Time) tea.Msg { return typingMsg{} })
}

func clockCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return clockMsg(t) })
}

func sparkleCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(t time.Time) tea.Msg { return sparkleMsg{} })
}

func (m model) Init() tea.Cmd {
	return tea.Batch(typingCmd(), clockCmd(), sparkleCmd())
}

// ── update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case typingMsg:
		if m.bioChars < totalBioChars(m.c()) {
			m.bioChars++
			return m, typingCmd()
		}

	case clockMsg:
		m.now = time.Time(msg)
		return m, clockCmd()

	case sparkleMsg:
		m.sparkleFrame++
		return m, sparkleCmd()

	case tea.KeyMsg:
		if m.page == pageGuestbookWrite {
			return m.updateWrite(msg)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "l":
			if m.lang == "en" {
				m.lang = "zh"
			} else {
				m.lang = "en"
			}

		case "esc", "backspace":
			if m.page != pageHome {
				m.page = pageHome
				m.guestScroll = 0
			}

		case "up", "k":
			switch m.page {
			case pageHome:
				m.selected = (m.selected + 3) % 4
			case pageGuestbook:
				if m.guestScroll > 0 {
					m.guestScroll--
				}
			}

		case "left", "h":
			if m.page == pageHome {
				m.selected = (m.selected + 3) % 4
			}

		case "down", "j":
			switch m.page {
			case pageHome:
				m.selected = (m.selected + 1) % 4
			case pageGuestbook:
				visible := m.visibleCount()
				if m.guestScroll < len(m.guestEntries)-visible {
					m.guestScroll++
				}
			}

		case "right":
			if m.page == pageHome {
				m.selected = (m.selected + 1) % 4
			}

		case "w":
			if m.page == pageGuestbook {
				m.page = pageGuestbookWrite
				m.nameInput.SetValue("")
				m.msgInput.SetValue("")
				m.activeInput = 0
				cmd := m.nameInput.Focus()
				m.msgInput.Blur()
				return m, cmd
			}

		case "enter", " ":
			if m.page == pageHome {
				newPage := page(m.selected + 1)
				m.page = newPage
				if newPage == pageGuestbook {
					m.guestEntries = fetchGuestbook()
					m.guestScroll = 0
				}
			}
		}
	}
	return m, nil
}

func (m model) updateWrite(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.page = pageGuestbook
		m.nameInput.Blur()
		m.msgInput.Blur()
		return m, nil

	case "tab":
		if m.activeInput == 0 {
			m.activeInput = 1
			m.nameInput.Blur()
			return m, m.msgInput.Focus()
		}
		m.activeInput = 0
		m.msgInput.Blur()
		return m, m.nameInput.Focus()

	case "shift+tab":
		if m.activeInput == 1 {
			m.activeInput = 0
			m.msgInput.Blur()
			return m, m.nameInput.Focus()
		}

	case "enter":
		if m.activeInput == 0 {
			m.activeInput = 1
			m.nameInput.Blur()
			return m, m.msgInput.Focus()
		}
		// submit
		name := strings.TrimSpace(m.nameInput.Value())
		message := strings.TrimSpace(m.msgInput.Value())
		if name != "" && message != "" {
			addGuestbookEntry(name, message)
			m.guestEntries = fetchGuestbook()
			m.guestScroll = 0
		}
		m.page = pageGuestbook
		m.nameInput.Blur()
		m.msgInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	if m.activeInput == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.msgInput, cmd = m.msgInput.Update(msg)
	}
	return m, cmd
}

func (m model) visibleCount() int {
	h := m.height - 8
	if h < 3 {
		return 1
	}
	return h / 3
}

// ── views ─────────────────────────────────────────────────────────────────────

func (m model) View() string {
	c := m.c()
	w := m.width
	if w < 80 {
		w = 80
	}

	switch m.page {
	case pageAbout:
		return m.viewDetail(c, w, c.TabAbout, m.viewAboutBody(c))
	case pageProjects:
		return m.viewDetail(c, w, c.TabProjects, m.viewProjectsBody(c))
	case pageContact:
		return m.viewDetail(c, w, c.TabContact, m.viewContactBody(c))
	case pageGuestbook:
		return m.viewDetailWithExtra(c, w, c.TabGuestbook, m.viewGuestbookBody(c), c.HelpWrite)
	case pageGuestbookWrite:
		return m.viewGuestbookWritePage(c, w)
	default:
		return m.viewHome(c, w)
	}
}

func renderSparkles(frame, width int) string {
	chars := []rune{'✦', '✧', '·', ' ', ' ', ' ', ' '}
	colors := []lipgloss.Color{colorPurple, colorCyan, colorPink, colorYellow, colorWhite}
	var b strings.Builder
	for i := 0; i < width; i++ {
		phase := (frame + i*2) % len(chars)
		ch := chars[phase]
		if ch == ' ' {
			b.WriteRune(' ')
		} else {
			col := colors[(i+frame/4)%len(colors)]
			b.WriteString(lipgloss.NewStyle().Foreground(col).Render(string(ch)))
		}
	}
	return b.String()
}

func indent(s string) string {
	return lipgloss.NewStyle().PaddingLeft(2).Render(s)
}

func helpBar(items ...string) string {
	return lipgloss.NewStyle().Foreground(colorDim).
		Render("  " + strings.Join(items, "   "))
}

func sep(w int) string {
	return lipgloss.NewStyle().Foreground(colorGray).
		Render(strings.Repeat("─", w-4))
}

// ── home ──────────────────────────────────────────────────────────────────────

func (m model) viewHome(c content, w int) string {
	avatar := strings.TrimRight(avatarArt, "\n")
	name := lipgloss.NewStyle().Foreground(colorPurple).
		Render(strings.TrimRight(nameArt, "\n"))

	csuLink := hyperlink("https://www.csu.edu.cn", "CSU")
	sub := lipgloss.NewStyle().Foreground(colorCyan).Italic(true).
		Render(csuLink + "  " + c.Role + "  ·  " + c.City)

	badge := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).
		Render("● " + c.Status)

	bio := m.renderBio(c)

	menuItems := []string{c.TabAbout, c.TabProjects, c.TabContact, c.TabGuestbook}
	var menuParts []string
	for i, item := range menuItems {
		if i == m.selected {
			menuParts = append(menuParts,
				lipgloss.NewStyle().Foreground(colorOrange).Bold(true).Render("❯ "+item))
		} else {
			menuParts = append(menuParts,
				lipgloss.NewStyle().Foreground(colorGray).Render(item))
		}
	}
	sep3 := lipgloss.NewStyle().Foreground(colorGray).Render("  ·  ")
	menu := strings.Join(menuParts, sep3)

	preview := m.viewMenuPreview(c)
	sparkles := renderSparkles(m.sparkleFrame, lipgloss.Width(name))

	rightCol := lipgloss.JoinVertical(lipgloss.Left,
		name, sparkles, sub, badge, "", bio, "", menu, "", preview,
	)

	combined := lipgloss.JoinHorizontal(lipgloss.Top,
		avatar,
		strings.Repeat(" ", 4),
		rightCol,
	)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		indent(combined),
		"",
		indent(sep(w)),
		m.homeHelpBar(c, w),
		"",
	)
}

func (m model) renderBio(c content) string {
	total := totalBioChars(c)
	var lines []string
	if m.bioChars >= total {
		for _, l := range c.Bio {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorWhite).Render(l))
		}
		return strings.Join(lines, "\n")
	}
	remaining := m.bioChars
	for _, l := range c.Bio {
		runes := []rune(l)
		if remaining <= 0 {
			break
		}
		n := min(remaining, len(runes))
		lines = append(lines, lipgloss.NewStyle().Foreground(colorWhite).Render(string(runes[:n])))
		remaining -= n
	}
	return strings.Join(lines, "\n")
}

func totalBioChars(c content) int {
	total := 0
	for _, l := range c.Bio {
		total += len([]rune(l))
	}
	return total
}

func (m model) viewMenuPreview(c content) string {
	dim := lipgloss.NewStyle().Foreground(colorDim)
	switch m.selected {
	case 0:
		return lipgloss.JoinVertical(lipgloss.Left,
			dim.Render("   "+c.Name),
			dim.Render(fmt.Sprintf("   %d skills · %s", len(c.Skills), c.City)),
		)
	case 1:
		var lines []string
		for _, p := range c.Projects {
			lines = append(lines, dim.Render("   · "+p.Name))
		}
		return strings.Join(lines, "\n")
	case 2:
		var lines []string
		for _, ct := range c.Contacts[:min(3, len(c.Contacts))] {
			lines = append(lines, dim.Render("   "+ct.Value))
		}
		return strings.Join(lines, "\n")
	case 3:
		if len(m.guestEntries) == 0 {
			return dim.Render("   " + c.GuestbookEmpty)
		}
		var lines []string
		for _, e := range m.guestEntries[:min(2, len(m.guestEntries))] {
			msg := e.Message
			if len([]rune(msg)) > 24 {
				msg = string([]rune(msg)[:24]) + "…"
			}
			lines = append(lines, dim.Render("   · "+e.Name+": "+msg))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

func (m model) homeHelpBar(c content, w int) string {
	left := lipgloss.NewStyle().Foreground(colorDim).
		Render("  " + strings.Join([]string{c.HelpNav, c.HelpOpen, c.HelpLang, c.HelpQuit}, "   "))
	visitor := lipgloss.NewStyle().Foreground(colorDim).
		Render(fmt.Sprintf("%s #%d", c.VisitorLabel, m.visitorNum))
	clock := lipgloss.NewStyle().Foreground(colorDim).
		Render(m.now.Format("15:04:05") + "  ")

	right := visitor + "   " + clock
	spacer := w - lipgloss.Width(left) - lipgloss.Width(right)
	if spacer < 1 {
		spacer = 1
	}
	return left + strings.Repeat(" ", spacer) + right
}

// ── detail pages ──────────────────────────────────────────────────────────────

func (m model) viewDetail(c content, w int, title, body string) string {
	return m.viewDetailWithExtra(c, w, title, body)
}

func (m model) viewDetailWithExtra(c content, w int, title, body string, extra ...string) string {
	heading := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).
		Render("── " + title + " " + strings.Repeat("─", max(0, w-len(title)-8)))

	helpItems := append(extra, c.HelpBack, c.HelpLang, c.HelpQuit)
	help := helpBar(helpItems...)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		indent(heading),
		"",
		indent(body),
		"",
		indent(sep(w)),
		help,
		"",
	)
}

func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── about ─────────────────────────────────────────────────────────────────────

func (m model) viewAboutBody(c content) string {
	label := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorYellow).Width(6).Render(s)
	}
	val := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorWhite).Render(s)
	}

	info := strings.Join([]string{
		label("Name") + val(c.Name),
		label("Role") + val(c.Role),
		label("City") + val(c.City),
	}, "\n")

	bioHeader := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("Bio")
	var bioLines []string
	for _, l := range c.Bio {
		bioLines = append(bioLines, "  "+lipgloss.NewStyle().Foreground(colorWhite).Render(l))
	}

	skillHeader := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(c.SkillsTitle)
	var skillLines []string
	for _, s := range c.Skills {
		skillLines = append(skillLines, "  "+lipgloss.NewStyle().Foreground(colorGreen).Render("▸ "+s))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		info, "",
		bioHeader,
		strings.Join(bioLines, "\n"), "",
		skillHeader,
		strings.Join(skillLines, "\n"),
	)
}

// ── projects ──────────────────────────────────────────────────────────────────

func (m model) viewProjectsBody(c content) string {
	var lines []string
	for i, p := range c.Projects {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(colorPurple).Bold(true).
				Render(fmt.Sprintf("%d. %s", i+1, p.Name)),
		)
		for _, dl := range strings.Split(p.Desc, "\n") {
			lines = append(lines, "   "+lipgloss.NewStyle().Foreground(colorWhite).Render(dl))
		}
		for _, link := range p.Links {
			linked := hyperlink(link, "→ "+link)
			lines = append(lines, "   "+lipgloss.NewStyle().Foreground(colorCyan).Render(linked))
		}
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// ── contact ───────────────────────────────────────────────────────────────────

func contactURL(val string) string {
	switch {
	case strings.HasPrefix(val, "github.com/"):
		return "https://" + val
	case strings.HasPrefix(val, "linkedin.com/"):
		return "https://" + val
	case strings.HasPrefix(val, "http"):
		return val
	case strings.Contains(val, "@") && !strings.HasPrefix(val, "@"):
		return "mailto:" + val
	default:
		return ""
	}
}

func (m model) viewContactBody(c content) string {
	var lines []string
	for _, ct := range c.Contacts {
		label := lipgloss.NewStyle().Foreground(colorYellow).Width(12).Render(ct.Label)
		display := ct.Value
		if url := contactURL(ct.Value); url != "" {
			display = hyperlink(url, ct.Value)
		}
		value := lipgloss.NewStyle().Foreground(colorCyan).Render(display)
		lines = append(lines, "  "+label+value)
	}
	lines = append(lines, "")
	lines = append(lines,
		lipgloss.NewStyle().Foreground(colorPink).Italic(true).
			Render("  ✦ Open to collaborations, internships & interesting conversations."),
	)
	return strings.Join(lines, "\n")
}

// ── guestbook ─────────────────────────────────────────────────────────────────

func (m model) viewGuestbookBody(c content) string {
	if len(m.guestEntries) == 0 {
		return lipgloss.NewStyle().Foreground(colorDim).Italic(true).Render(c.GuestbookEmpty)
	}

	visible := m.visibleCount()
	start := m.guestScroll
	end := start + visible
	if end > len(m.guestEntries) {
		end = len(m.guestEntries)
	}

	var lines []string

	if start > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(colorDim).Render(fmt.Sprintf("  ▲ %d more above", start)),
			"",
		)
	}

	for _, e := range m.guestEntries[start:end] {
		nameStr := lipgloss.NewStyle().Foreground(colorPurple).Bold(true).Render(e.Name)
		dateStr := lipgloss.NewStyle().Foreground(colorGray).Render(e.CreatedAt.Format("2006-01-02"))
		msgStr := lipgloss.NewStyle().Foreground(colorWhite).Render(e.Message)
		lines = append(lines, nameStr+"  "+dateStr)
		lines = append(lines, "  "+msgStr)
		lines = append(lines, "")
	}

	remaining := len(m.guestEntries) - end
	if remaining > 0 {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(colorDim).Render(fmt.Sprintf("  ▼ %d more below", remaining)),
		)
	}

	return strings.Join(lines, "\n")
}

func (m model) viewGuestbookWritePage(c content, w int) string {
	heading := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).
		Render("── " + c.TabGuestbook + " " + strings.Repeat("─", max(0, w-len(c.TabGuestbook)-8)))

	focusedLabel := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(s)
	}
	dimLabel := func(s string) string {
		return lipgloss.NewStyle().Foreground(colorGray).Render(s)
	}

	var nameLabel, msgLabel string
	if m.activeInput == 0 {
		nameLabel = focusedLabel(c.GuestbookNameLabel)
		msgLabel = dimLabel(c.GuestbookMsgLabel)
	} else {
		nameLabel = dimLabel(c.GuestbookNameLabel)
		msgLabel = focusedLabel(c.GuestbookMsgLabel)
	}

	body := lipgloss.JoinVertical(lipgloss.Left,
		nameLabel,
		"  "+m.nameInput.View(),
		"",
		msgLabel,
		"  "+m.msgInput.View(),
	)

	help := helpBar(c.HelpSubmit, c.HelpBack, c.HelpQuit)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		indent(heading),
		"",
		indent(body),
		"",
		indent(sep(w)),
		help,
		"",
	)
}

// ── SSH server ────────────────────────────────────────────────────────────────

func main() {
	if err := initDB("./data/portfolio.db"); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	s, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%s", host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
		),
	)
	if err != nil {
		log.Fatalf("could not create server: %v", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Starting SSH server on %s:%s", host, port)
	go func() {
		if err := s.ListenAndServe(); err != nil && err != ssh.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown error: %v", err)
	}
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	_, _, active := s.Pty()
	if !active {
		fmt.Fprintln(s, "No PTY requested, please use: ssh -t ...")
		return nil, nil
	}
	renderer := bubbletea.MakeRenderer(s)
	renderer.SetColorProfile(termenv.TrueColor)
	lipgloss.SetDefaultRenderer(renderer)

	ip := extractIP(s.RemoteAddr().String())
	visitorNum := registerVisitor(ip)
	entries := fetchGuestbook()

	return initialModel(visitorNum, entries), []tea.ProgramOption{tea.WithAltScreen()}
}
