# SSH Portfolio — 架构与决策日志

## 设计思路

### 架构：SSH → PTY → Bubble Tea

```
用户 ssh 连接
    │
    ▼
Wish SSH Server（监听 :106）
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

## 本地启动流程

```bash
# 热重载（日常开发用）
air

# 连接测试
ssh -p 222 localhost
```

> 首次启动会在 `.ssh/id_ed25519` 自动生成主机密钥，无需手动操作。
> 若遇到 `Connection refused`，确认服务在运行：`lsof -i :106`

## 依赖版本说明

`charmbracelet/x/input` 与 `charmbracelet/x/ansi` 存在版本兼容性问题，需保持同步升级。
出现 `undefined: ansi.CsiSequence` 类错误时，执行：

```bash
go get github.com/charmbracelet/bubbletea@latest \
       github.com/charmbracelet/lipgloss@latest \
       github.com/charmbracelet/wish@latest
```

## 部署方式（Ubuntu VPS + Docker）

### 1. 上传项目代码

在**本机**执行：

```bash
rsync -avz \
  --filter=':- .gitignore' \
  --exclude='.git/' \
  --exclude='memory/' \
  --exclude='.ssh/' \
  --delete \
  <local-project-path>/ <server>:~/ssh-portfolio/
```

> 完整命令（含本机路径和 SSH alias）见 `local/ops.md`。
> `.ssh/` 必须排除：服务器的主机密钥由容器首次启动自动生成并持久化在 volume 里，rsync 不能覆盖它，否则每次部署后客户端会报 host key 变更警告。

### 2. 运行部署脚本

在**服务器**上执行：

```bash
cd ~/ssh-portfolio
bash deploy.sh
```

脚本会自动完成：安装 Docker（如未安装）、构建镜像、替换旧容器、配置 UFW。
端口从 `Dockerfile` 的 `EXPOSE` 自动读取，无需手动指定。

如需覆盖参数：

```bash
PORT=106 CONTAINER_NAME=my-portfolio bash deploy.sh
```

### 3. 开放云服务商安全组（必须）

云服务商（腾讯云/阿里云等）有独立的安全组，和 UFW 是两层，缺一不可。在控制台手动添加入站规则：

> - 协议：TCP
> - 端口：与 `Dockerfile` 中 `EXPOSE` 一致
> - 来源：0.0.0.0/0

> **排查 `Operation timed out`**：如果容器运行正常（`docker ps` 显示 Up）、UFW 已放行，但仍然连不上，99% 是安全组没配。

### 4. 验证

```bash
ssh -p 106 ssh.mornqing.com
```

### 日常更新流程

```bash
# 本机：同步新代码
rsync -avz \
  --filter=':- .gitignore' \
  --exclude='.git/' \
  --exclude='memory/' \
  --exclude='.ssh/' \
  --delete \
  <local-project-path>/ <server>:~/ssh-portfolio/

# 服务器：重新构建并重启
cd ~/ssh-portfolio
bash deploy.sh
```

### 常用命令

```bash
docker logs ssh-portfolio -f      # 实时日志
docker ps                          # 确认容器运行状态
docker stop ssh-portfolio          # 停止
docker restart ssh-portfolio       # 重启
```

## 项目文件结构

```
ssh-portfolio/
├── CLAUDE.md              # 本文件（决策日志）
├── main.go                # 全部逻辑：SSH server + TUI model + 内容
├── ascii/
│   ├── avatar.txt         # ASCII 头像（占位符，替换为自己的）
│   └── name.txt           # ASCII 名字大字（QC block letters）
├── go.mod
├── go.sum
├── Dockerfile
├── deploy.sh              # 服务器部署脚本（端口从 Dockerfile 自动读取）
└── local/                 # 个人运维笔记，不上传（已加入 .gitignore）
    └── ops.md             # 服务器 IP、SSH alias、完整 rsync 命令等
```
