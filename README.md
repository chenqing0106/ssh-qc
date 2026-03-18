# ssh-portfolio

一个基于终端的个人主页，通过 SSH 即可访问，不需要浏览器。

```bash
ssh -p 106 ssh.mornqing.com
```

任何人在终端运行以上命令即可访问，无需安装任何东西。

使用 [Wish](https://github.com/charmbracelet/wish) + [Bubble Tea](https://github.com/charmbracelet/bubbletea) 构建，Go 语言编写。

## 功能

- 彩色 ASCII 头像 + 大字名称
- 连接时 Bio 打字机动画
- 右下角实时时钟
- 名字下方闪烁动画
- 首页菜单悬停预览
- 三个详情页：**关于**、**项目**、**联系方式**
- 中英双语实时切换（`l`）
- 可点击超链接（支持 iTerm2、Kitty、GNOME Terminal 等）

## 按键

| 按键 | 操作 |
|------|------|
| `↑↓` / `jk` / `←→` / `hl` | 菜单导航 |
| `enter` | 进入页面 |
| `esc` / `backspace` | 返回首页 |
| `l` | 切换语言（中 ↔ 英） |
| `q` / `ctrl+c` | 退出 |

## 技术栈

| 组件 | 库 |
|------|-----|
| SSH 服务器 | [Wish](https://github.com/charmbracelet/wish) |
| TUI 框架 | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| 终端样式 | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| 语言 | Go 1.22+ |

## 本地开发

```bash
# 热重载（需要安装 air）
air

# 或手动构建运行
go build -o ssh-portfolio . && ./ssh-portfolio

# 连接测试
ssh -p 106 localhost
```

首次运行会自动在 `.ssh/id_ed25519` 生成主机密钥。

## 自定义内容

所有文案在 `main.go` 中的 `en` 和 `zh` 变量里，修改名字、Bio、项目、联系方式后重新构建即可。

ASCII 资源文件：
- `ascii/avatar.txt` — 彩色 ASCII 头像（用 `ascii-image-converter` 生成）
- `ascii/name.txt` — 大字名称

## 部署

需要支持持久 TCP 连接的平台（不支持 Vercel / Netlify）。

**任意 VPS**（阿里云、腾讯云等）：

```bash
# 上传代码后，在服务器上执行
bash deploy.sh
```

脚本自动处理 Docker 安装、镜像构建、容器替换、UFW 配置。端口从 `Dockerfile` 的 `EXPOSE` 读取。

> 云服务商的安全组需要手动在控制台放行对应端口（入站 TCP）。

## 项目结构

```
ssh-portfolio/
├── main.go          # 全部逻辑：SSH 服务器 + TUI + 内容
├── ascii/
│   ├── avatar.txt   # ASCII 头像
│   └── name.txt     # 大字名称
├── Dockerfile
└── deploy.sh        # VPS 一键部署脚本
```
