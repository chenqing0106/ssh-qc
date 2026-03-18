package main

import (
	_ "embed"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/muesli/termenv"
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
)

// ── content ───────────────────────────────────────────────────────────────────

type content struct {
	TabAbout    string `yaml:"tab_about"`
	TabProjects string `yaml:"tab_projects"`
	TabContact  string `yaml:"tab_contact"`

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

	HelpNav  string `yaml:"help_nav"`
	HelpOpen string `yaml:"help_open"`
	HelpBack string `yaml:"help_back"`
	HelpLang string `yaml:"help_lang"`
	HelpQuit string `yaml:"help_quit"`
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

// ── model ─────────────────────────────────────────────────────────────────────

type model struct {
	page     page
	selected int // home menu: 0=About 1=Projects 2=Contact
	lang     string
	width    int
	height   int
	bioChars     int       // typing animation: chars revealed so far
	now          time.Time // real-time clock
	sparkleFrame int       // sparkle animation frame counter
}

func initialModel() model {
	return model{page: pageHome, selected: 0, lang: "en", now: time.Now()}
}

func (m model) c() content {
	if m.lang == "zh" {
		return zh
	}
	return en
}

// ── tick messages ──────────────────────────────────────────────────────────────

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
			}
		case "up", "k", "left", "h":
			if m.page == pageHome {
				m.selected = (m.selected + 2) % 3
			}
		case "down", "j", "right":
			if m.page == pageHome {
				m.selected = (m.selected + 1) % 3
			}
		case "enter", " ":
			if m.page == pageHome {
				m.page = page(m.selected + 1)
			}
		}
	}
	return m, nil
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
	default:
		return m.viewHome(c, w)
	}
}

// renderSparkles renders a full-width animated row of sparkle characters
// that fills exactly `width` visible columns beneath the name art.
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

// indent adds 2 spaces to the left of every line in a multi-line block.
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
	// Left column: avatar (keep its own ANSI colors, don't override)
	avatar := strings.TrimRight(avatarArt, "\n")
	// Right column
	name := lipgloss.NewStyle().Foreground(colorPurple).
		Render(strings.TrimRight(nameArt, "\n"))

	csuLink := hyperlink("https://www.csu.edu.cn", "CSU")
	sub := lipgloss.NewStyle().Foreground(colorCyan).Italic(true).
		Render(csuLink + "  " + c.Role + "  ·  " + c.City)

	badge := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).
		Render("● " + c.Status)

	bio := m.renderBio(c)

	menuItems := []string{c.TabAbout, c.TabProjects, c.TabContact}
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

	gap := 4
	combined := lipgloss.JoinHorizontal(lipgloss.Top,
		avatar,
		strings.Repeat(" ", gap),
		rightCol,
	)

	help := m.homeHelpBar(c, w)

	return lipgloss.JoinVertical(lipgloss.Left,
		"",
		indent(combined),
		"",
		indent(sep(w)),
		help,
		"",
	)
}

// renderBio renders bio text with typing animation applied.
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

// viewMenuPreview shows a dimmed preview of the selected menu item.
func (m model) viewMenuPreview(c content) string {
	dim := lipgloss.NewStyle().Foreground(colorDim)
	switch m.selected {
	case 0: // About
		return lipgloss.JoinVertical(lipgloss.Left,
			dim.Render("   "+c.Name),
			dim.Render(fmt.Sprintf("   %d skills · %s", len(c.Skills), c.City)),
		)
	case 1: // Projects
		var lines []string
		for _, p := range c.Projects {
			lines = append(lines, dim.Render("   · "+p.Name))
		}
		return strings.Join(lines, "\n")
	case 2: // Contact
		var lines []string
		for _, ct := range c.Contacts[:min(3, len(c.Contacts))] {
			lines = append(lines, dim.Render("   "+ct.Value))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

// homeHelpBar renders the help bar with a right-aligned clock.
func (m model) homeHelpBar(c content, w int) string {
	left := lipgloss.NewStyle().Foreground(colorDim).
		Render("  " + strings.Join([]string{c.HelpNav, c.HelpOpen, c.HelpLang, c.HelpQuit}, "   "))
	clock := lipgloss.NewStyle().Foreground(colorDim).
		Render(m.now.Format("15:04:05") + "  ")

	spacer := w - lipgloss.Width(left) - lipgloss.Width(clock)
	if spacer < 1 {
		spacer = 1
	}
	return left + strings.Repeat(" ", spacer) + clock
}

// ── detail page wrapper ───────────────────────────────────────────────────────

func (m model) viewDetail(c content, w int, title, body string) string {
	heading := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).
		Render("── " + title + " " + strings.Repeat("─", max(0, w-len(title)-8)))

	help := helpBar(c.HelpBack, c.HelpLang, c.HelpQuit)

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

// hyperlink wraps text with OSC 8 terminal hyperlink escape codes.
// Terminals that support it (iTerm2, Kitty, GNOME Terminal, etc.) render
// a clickable link; others display the text as-is.
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

// ── SSH server ────────────────────────────────────────────────────────────────

func main() {
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
	return initialModel(), []tea.ProgramOption{tea.WithAltScreen()}
}
