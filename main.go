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
)

//go:embed ascii/avatar.txt
var avatarArt string

//go:embed ascii/name.txt
var nameArt string

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
	tabAbout    string
	tabProjects string
	tabContact  string

	name   string
	role   string
	city   string
	status string
	bio    []string

	skillsTitle string
	skills      []string

	projectsTitle string
	projects      []project

	contactTitle string
	contacts     []contact

	helpNav  string
	helpOpen string
	helpBack string
	helpLang string
	helpQuit string
}

type project struct {
	name  string
	desc  string
	links []string
}

type contact struct {
	label string
	value string
}

var en = content{
	tabAbout: "About", tabProjects: "Projects", tabContact: "Contact",
	name: "QingChen (卿晨)", role: "Software Engineering Student", city: "Changsha, China",
	status: "open to work",
	bio: []string{
		"Aspiring builder who weaves a wider world through code！",
		"exploring the intersection of technology and humanities.",
		"Like a mushroom thriving in post-apocalyptic ruins —",
		"slowly expanding a decentralized, anti-monopoly colony.",
	},
	skillsTitle: "Skills & Expertise",
	skills: []string{
		"React / Next.js 15 / TypeScript",
		"LLM Integration (Claude / Gemini / Qwen API)",
		"Canvas API / Web Audio API / Tone.js",
		"Node.js / RESTful API",
		"Web3 concepts & Solidity basics",
		"AI-Native Development (Claude Code, Cursor)",
		"Visual arts, humanities & cultural exploration",
	},
	projectsTitle: "Selected Projects",
	projects: []project{
		{
			name: "Chain-Garden",
			desc: "AI Native generative art platform — turns emotions into living\nplants with unique visuals and sound, powered by Prompt\nChains and Canvas rendering. Hackathon Best Creativity Award.",
			links: []string{
				"https://chain-garden-zetachain.vercel.app",
				"https://github.com/chenqing0106/chain-garden",
			},
		},
	},
	contactTitle: "Get in Touch",
	contacts: []contact{
		{"GitHub", "github.com/chenqing0106"},
		{"Email", "qingchen060106@gmail.com"},
		{"Twitter/X", "@qingchen060106"},
		{"Telegram", "@qingchen060106"},
		{"LinkedIn", "linkedin.com/in/sylva-qing"},
	},
	helpNav: "←→ select", helpOpen: "enter open", helpBack: "esc back", helpLang: "l lang", helpQuit: "q quit",
}

var zh = content{
	tabAbout: "关于", tabProjects: "项目", tabContact: "联系",
	name: "卿晨 (QingChen)", role: "软件工程在读学生", city: "中国·长沙",
	status: "求职中",
	bio: []string{
		"想成为世界的 builder，用编程去编织更广阔的世界，",
		"致力于探索技术与人文的交叉点。",
		"成为一颗末日废墟下生存的菌菇，",
		"慢慢延展构造去中心化、反垄断的世界。",
	},
	skillsTitle: "技能 / 专长",
	skills: []string{
		"React / Next.js 15 / TypeScript",
		"LLM 集成（Claude / Gemini / Qwen API）",
		"Canvas API / Web Audio API / Tone.js",
		"Node.js / RESTful API",
		"Web3 概念与 Solidity 基础",
		"AI 原生开发（Claude Code, Cursor）",
		"日常关注视觉艺术、人文与文化话题",
	},
	projectsTitle: "精选项目",
	projects: []project{
		{
			name: "Chain-Garden",
			desc: "AI 原生生成艺术平台——将情绪转化为有生命的植物，\n融合独特视觉与音效，基于 Prompt Chains 与 Canvas 渲染。\n黑客松最佳创意奖。",
			links: []string{
				"https://chain-garden-zetachain.vercel.app",
				"https://github.com/chenqing0106/chain-garden",
			},
		},
	},
	contactTitle: "联系方式",
	contacts: []contact{
		{"GitHub", "github.com/chenqing0106"},
		{"邮箱", "qingchen060106@gmail.com"},
		{"Twitter/X", "@qingchen060106"},
		{"Telegram", "@qingchen060106"},
		{"LinkedIn", "linkedin.com/in/sylva-qing"},
	},
	helpNav: "←→ 选择", helpOpen: "enter 进入", helpBack: "esc 返回", helpLang: "l 语言", helpQuit: "q 退出",
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
		return m.viewDetail(c, w, c.tabAbout, m.viewAboutBody(c))
	case pageProjects:
		return m.viewDetail(c, w, c.tabProjects, m.viewProjectsBody(c))
	case pageContact:
		return m.viewDetail(c, w, c.tabContact, m.viewContactBody(c))
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
		Render(csuLink + "  " + c.role + "  ·  " + c.city)

	badge := lipgloss.NewStyle().Foreground(colorGreen).Bold(true).
		Render("● " + c.status)

	bio := m.renderBio(c)

	menuItems := []string{c.tabAbout, c.tabProjects, c.tabContact}
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
		for _, l := range c.bio {
			lines = append(lines, lipgloss.NewStyle().Foreground(colorWhite).Render(l))
		}
		return strings.Join(lines, "\n")
	}
	remaining := m.bioChars
	for _, l := range c.bio {
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
	for _, l := range c.bio {
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
			dim.Render("   "+c.name),
			dim.Render(fmt.Sprintf("   %d skills · %s", len(c.skills), c.city)),
		)
	case 1: // Projects
		var lines []string
		for _, p := range c.projects {
			lines = append(lines, dim.Render("   · "+p.name))
		}
		return strings.Join(lines, "\n")
	case 2: // Contact
		var lines []string
		for _, ct := range c.contacts[:min(3, len(c.contacts))] {
			lines = append(lines, dim.Render("   "+ct.value))
		}
		return strings.Join(lines, "\n")
	}
	return ""
}

// homeHelpBar renders the help bar with a right-aligned clock.
func (m model) homeHelpBar(c content, w int) string {
	left := lipgloss.NewStyle().Foreground(colorDim).
		Render("  " + strings.Join([]string{c.helpNav, c.helpOpen, c.helpLang, c.helpQuit}, "   "))
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

	help := helpBar(c.helpBack, c.helpLang, c.helpQuit)

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
		label("Name") + val(c.name),
		label("Role") + val(c.role),
		label("City") + val(c.city),
	}, "\n")

	bioHeader := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render("Bio")
	var bioLines []string
	for _, l := range c.bio {
		bioLines = append(bioLines, "  "+lipgloss.NewStyle().Foreground(colorWhite).Render(l))
	}

	skillHeader := lipgloss.NewStyle().Foreground(colorCyan).Bold(true).Render(c.skillsTitle)
	var skillLines []string
	for _, s := range c.skills {
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
	for i, p := range c.projects {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(colorPurple).Bold(true).
				Render(fmt.Sprintf("%d. %s", i+1, p.name)),
		)
		for _, dl := range strings.Split(p.desc, "\n") {
			lines = append(lines, "   "+lipgloss.NewStyle().Foreground(colorWhite).Render(dl))
		}
		for _, link := range p.links {
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
	for _, ct := range c.contacts {
		label := lipgloss.NewStyle().Foreground(colorYellow).Width(12).Render(ct.label)
		display := ct.value
		if url := contactURL(ct.value); url != "" {
			display = hyperlink(url, ct.value)
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
	return initialModel(), []tea.ProgramOption{tea.WithAltScreen()}
}
