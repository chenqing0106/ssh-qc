# SSH Portfolio — 项目背景与技术方案

## 项目目标

构建一个基于终端的个人网站，用户通过 `ssh your-domain.com` 即可访问，看到的是交互式 TUI 界面而非普通 shell。参考：ssh.moriliu.com、ssh hi.zachkrall.com。

## 技术栈

| 组件 | 技术 |
|------|------|
| SSH 服务器 | [Wish](https://github.com/charmbracelet/wish) (Charm) |
| TUI 框架 | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| 终端样式 | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| 语言 | Go |
| 部署 | Fly.io（免费额度） |

## 设计思路

### 架构：SSH → PTY → Bubble Tea

```
用户 ssh 连接
    │
    ▼
Wish SSH Server（监听 :23234）
    │  检查 PTY 是否存在，无 PTY 则拒绝并提示
    ▼
teaHandler → 创建 Bubble Tea model
    │
    ▼
Bubble Tea 事件循环（运行在 SSH session 的 PTY 上）
    │  WindowSizeMsg → 动态适配终端宽度
    │  KeyMsg        → 页面切换 / 语言切换 / 退出
    ▼
View() 渲染 → Lip Gloss 样式化输出 → 回写到 SSH 连接
```

### 核心设计决策

1. **单文件 `main.go`**：所有逻辑集中在一个文件，便于理解和修改内容。
2. **双语内容结构**：`content` struct 存储所有文案，`en`/`zh` 两个实例，通过 `l` 键切换，无需重启。
3. **动态宽度适配**：响应 `tea.WindowSizeMsg`，`width` 最小值兜底 80 列，防止在窄终端下布局错乱。
4. **PTY 强制检查**：`teaHandler` 中检查 `s.Pty()`，非 PTY 连接（如脚本/管道）给出友好提示而非崩溃。
5. **优雅关闭**：监听 `SIGINT`/`SIGTERM`，`s.Shutdown()` 有 30 秒超时，允许已连接会话自然结束。
6. **主机密钥持久化**：`WithHostKeyPath(".ssh/id_ed25519")` — 首次运行自动生成，重启后客户端不会报 host key 变更警告。

### 布局结构（View 层）

Home 页：左侧 ASCII 头像，右侧上方 ASCII 名字 + 副标题，中间 Bio，下方三个可选菜单项 + 语言切换。

```
  [ascii/avatar.txt]   [ascii/name.txt — QC 大字]
                       Software Engineering Student · Changsha

                       Bio 四行...

                       ❯  About       ← ↑↓/jk 选择，enter 进入
                          Projects
                          Contact

                       ╭EN╮ ╭ZH╮

  ─────────────────────────────────────────────
  ↑↓ select   enter open   l lang   q quit
```

详情页（About / Projects / Contact）：无边框，纯文字排版，`esc` 返回 Home。

```
  ── About ──────────────────────────────

  内容...

  ─────────────────────────────────────
  esc back   l lang   q quit
```

### ASCII 资源文件（`//go:embed` 嵌入）

| 文件 | 用途 | 说明 |
|------|------|------|
| `ascii/avatar.txt` | 左侧头像 | 彩色 ANSI 输出，不能被 lipgloss 颜色覆盖 |
| `ascii/name.txt` | 右侧名字大字 | `QC` block letter，可单独修改字体/样式 |

修改后无需改 `main.go`，重新 `go build` 即生效。

### ASCII 头像生成方式

使用 `ascii-image-converter` 工具：

```bash
ascii-image-converter resources/avater-2.jpeg -W 50 --color > ascii/avatar.txt
```

关键参数选择：
- `-W 50`：50字符宽，人脸清晰可辨的最低舒适尺寸；低于40字符五官模糊
- `--color`：输出 ANSI 24-bit 彩色转义码；文件中存的是原始 escape codes
- 高度自动计算（字符比例约 2:1，50宽 → 约25行高）

**注意**：头像渲染时不能用 `lipgloss.NewStyle().Foreground()` 包裹，否则会覆盖 ANSI 颜色。直接用 `strings.TrimRight(avatarArt, "\n")` 输出。

### 页面导航逻辑

```
pageHome  --enter--> pageAbout / pageProjects / pageContact
pageAbout/Projects/Contact  --esc/backspace--> pageHome
任意页面  --l--> 切换语言（实时）
任意页面  --q/ctrl+c--> 退出
```

### 样式色盘

| 变量 | 颜色 | 用途 |
|------|------|------|
| `colorPurple` | `#C084FC` | Avatar、名字、激活菜单项、项目名 |
| `colorCyan` | `#67E8F9` | 副标题、章节标题、链接、详情页 heading |
| `colorGreen` | `#86EFAC` | 技能列表 |
| `colorYellow` | `#FDE68A` | 字段标签 |
| `colorPink` | `#F9A8D4` | Contact 页底部引导语 |
| `colorGray` | `#6B7280` | 非激活菜单项、分隔符、dimmed help 文字 |

### 去除边框的原因

Lip Gloss border + width 计算复杂，`Width()` 设置的是内容区宽度，加上 padding 和 border 后总宽度难以精确控制，导致布局溢出。现改为纯文字排版 + `indent()` 函数（`PaddingLeft(2)`）统一管理左边距。

## 功能

- 左侧彩色 ASCII 头像（`ascii/avatar.txt`）
- 右侧 ASCII 名字大字（`ascii/name.txt`）
- Home 菜单导航：`↑↓` / `jk` 选择，`enter` 进入详情页
- 菜单悬停预览：选中某项时在菜单下方显示该页内容摘要
- Bio 打字机动画：首次连接时逐字符显示（20ms/字符）
- 右下角实时时钟（每秒刷新）
- 三个详情页：**About**、**Projects**、**Contact**
- 中英双语实时切换（`l`）
- 详情页 `esc` 返回 Home

## 交互拓展备忘（待做）

| 优先级 | 功能             | 描述                                       |
| --- | -------------- | ---------------------------------------- |
| 高   | 访客计数器          | 每次连接 +1，持久化到文件，首页显示 "You are visitor #N" |
| 中   | 状态标签           | 名字旁显示 `● open to work` 等彩色徽章             |
| 中   | 随机 tagline     | 每次连接随机一句话，显示在 bio 下方                     |
| 低   | Konami Code 彩蛋 | ↑↑↓↓←→←→ 触发矩阵雨动画                         |
| 低   | 隐藏命令彩蛋         | 特定按键序列显示隐藏内容                             |

## 本地启动流程

```bash
# 1. 构建
go build -o ssh-portfolio .

# 2. 启动服务（后台运行）
./ssh-portfolio &

# 3. 连接测试
ssh -p 23234 localhost

# 4. 停止服务
kill $(pgrep ssh-portfolio)
```

> 首次启动会在 `.ssh/id_ed25519` 自动生成主机密钥，无需手动操作。
>
> 若遇到 `Connection refused`，确认服务确实在运行：`lsof -i :23234`

## 依赖版本说明

`charmbracelet/x/input` 与 `charmbracelet/x/ansi` 存在版本兼容性问题，需保持同步升级。
出现 `undefined: ansi.CsiSequence` 类错误时，执行：

```bash
go get github.com/charmbracelet/bubbletea@latest \
       github.com/charmbracelet/lipgloss@latest \
       github.com/charmbracelet/wish@latest
```

## 部署方式（Fly.io）

用户侧只需：
1. 注册 [Fly.io](https://fly.io) 账号
2. 安装 `flyctl`
3. 按指引执行几条命令

## 项目文件结构

```
ssh-portfolio/
├── CLAUDE.md              # 本文件（决策日志）
├── content.md             # 个人内容参考（已写入 main.go）
├── main.go                # 全部逻辑：SSH server + TUI model + 内容
├── ascii/
│   ├── avatar.txt         # ASCII 头像（占位符，替换为自己的）
│   └── name.txt           # ASCII 名字大字（QC block letters）
├── go.mod
├── go.sum
├── Dockerfile
└── fly.toml
```
