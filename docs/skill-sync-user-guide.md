# Skill Sync 快速上手指南

`skill-sync` 用来把同一份 skill 同步到多个 Agent 的本地 skill 目录，并且可以和团队的 Nacos Skill Registry 保持同步。它的目标不是让你反复复制目录，而是让多个 Agent 通过软链接使用一份中心副本，在有冲突时给出少量、安全、统一的选择。

本文里的示例使用 `nacos-cli`。如果你是在源码仓库里测试，把命令替换成 `./build/nacos-cli` 即可。

## 先理解这个模型

Skill Sync 有三个位置：

| 位置 | 作用 |
| --- | --- |
| Agent skill 目录 | Codex、Claude、Qoder 等工具实际读取 skill 的地方 |
| 本地中心仓库 | 默认在 `~/.nacos-cli/skill-repo`，`skill-sync` 会把 skill 放在这里 |
| Nacos | 团队共享的远端 skill registry，通常跟踪 `latest` label |

同步成功后，每个 Agent 目录里的同名 skill 通常会变成指向中心仓库的软链接。这样你编辑任意一个 Agent 看到的 skill，本质上是在编辑同一份内容。

默认会自动发现这些目录：

| Agent 名称 | 默认路径 |
| --- | --- |
| `codex` | `~/.codex/skills` |
| `claude` | `~/.claude/skills` |
| `qoder` | `~/.qoder/skills` |
| `qoderwork` | `~/.qoderwork/skills` |
| `cursor` | `~/.cursor/skills` |
| `kiro` | `~/.kiro/skills` |
| `lingma` | `~/.lingma/skills` |
| `copaw` | `~/.copaw/skill_pool` |
| `openclaw` | `~/.openclaw/skills` |
| `agents` | `~/.agents/skills` |
| `default` | `~/.skills` |

如果你有其他 Agent 目录，先注册一次：

```bash
nacos-cli skill-sync agent add my-agent ~/.my-agent/skills
nacos-cli skill-sync agent list
```

本地目录只有包含 `SKILL.md` 时，才会被识别为一个可用 skill 来源。

## 第一次使用

如果你要和 Nacos 同步，先确保当前 profile 能连接到 Nacos。第一次运行时，CLI 会检测 profile 并询问是否使用 Nacos mode：

```text
Detected configured profile: team
Use Nacos mode? [Y/n]:
```

如果是脚本或 Agent 调用，不要依赖这个交互，直接显式传 profile：

```bash
nacos-cli skill-sync add pdf --profile team --non-interactive
```

如果本机没有可用 profile，`skill-sync` 会进入 local mode。local mode 不访问 Nacos，只负责把本地中心仓库和多个 Agent 目录统一起来。

如果之前已经选择过 Nacos mode，后来只想使用本地同步，不要通过删除 profile 配置文件来切换模式。直接执行：

```bash
nacos-cli skill-sync mode local
```

这个命令会把当前 sync profile 的模式切回 local，并停止已有的 Nacos sync daemon。后续不带 `--profile` 的 `skill-sync add` / `start` 不会访问 Nacos。需要重新使用 Nacos 时，再切回：

```bash
nacos-cli skill-sync mode nacos --profile team
```

## 最常见路径：从团队安装一个 skill

假设团队已经在 Nacos 上发布了 `pdf`：

```bash
nacos-cli skill-sync add pdf --profile team
```

正常情况下它会：

1. 从 Nacos 拉取 `pdf` 的 `latest` 版本。
2. 写入 `~/.nacos-cli/skill-repo/pdf`。
3. 把已发现的 Agent 目录都链接到这份中心副本。
4. 把状态记录为 `Synced`。

之后启动同步守护进程：

```bash
nacos-cli skill-sync start --profile team
```

`start` 会先做一次初始同步，然后启动后台 daemon。你可以随时看状态：

```bash
nacos-cli skill-sync status
```

如果想在当前终端观察日志：

```bash
nacos-cli skill-sync start --profile team --foreground
```

## 本地已经有同名 skill 时怎么选

如果 Nacos 上有 `pdf`，本地多个 Agent 目录也有 `pdf`，而且内容不同，`add` 会让你选择来源：

```text
Choose source:
  [1] Use Nacos version
  [2] Use claude version
  [3] Use codex version
  [4] Exit
```

推荐按这个原则选：

| 你想要的结果 | 选择 |
| --- | --- |
| 直接使用团队版本 | `Use Nacos version` |
| 本地某个 Agent 的版本才是正确版本 | 选择那个 Agent |
| 还没判断清楚 | `Exit` |

选择 Nacos 版本时，CLI 会用远端内容作为中心副本，并把 Agent 目录链接过去。原来不同的本地目录会先备份到对应 Agent 目录下的 `.skill-sync-backup/`。

选择某个 Agent 版本时，CLI 会把这个本地版本提升为中心副本，并把状态标记为 `Local changes`。这表示“本地内容和 Nacos 不一致，但这是你明确选择的本地版本”。如果自动上传开启，daemon 后续会尝试把它上传成 Nacos draft；如果自动上传关闭，它会一直留在本地，直到你手动上传。

## start 遇到冲突时怎么处理

`start` 的冲突选择更少，适合批量启动：

```text
Choose how to continue:
  [1] Use Nacos version for all conflicts
  [2] Record and skip conflicts
  [3] Exit
```

建议：

| 场景 | 选择 |
| --- | --- |
| 你确定团队版本应该覆盖本地 | `Use Nacos version for all conflicts` |
| 你不想让启动过程改动冲突 skill | `Record and skip conflicts` |
| 你需要先人工检查 | `Exit` |

非交互 `start` 遇到冲突时会记录并跳过，不会自动覆盖本地内容。

## 冲突后如何 resolve

当状态是 `Conflict` 时，先看状态：

```bash
nacos-cli skill-sync status
```

然后只处理冲突的 skill：

```bash
nacos-cli skill-sync resolve pdf
```

在 Nacos mode 下，`resolve` 的交互和 `add` 保持一致：

```text
Choose source:
  [1] Use Nacos version
  [2] Use codex version
  [3] Exit
```

选择 Nacos 版本会把本地恢复到团队版本并标记为 `Synced`。选择 Agent 版本会把该本地版本提升到中心仓库，并标记为 `Local changes`，后续是否上传由自动上传配置决定。

## 本地修改和自动上传

在 Nacos mode 下，自动上传默认开启。你本地修改一个已同步 skill 后，daemon 会在连续两次轮询看到同一个内容 hash 后再上传，避免刚编辑到一半就上传。

典型状态流转是：

```text
Synced -> Local changes -> Uploaded -> Synced
```

含义如下：

| 状态 | 代表什么 | 你通常需要做什么 |
| --- | --- | --- |
| `Synced` | 本地和 Nacos 当前跟踪版本一致 | 不需要处理 |
| `Local changes` | 本地内容已变更，还没上传成功 | 等待自动上传，或手动 `skill-upload` |
| `Uploaded` | 本地内容已上传为 Nacos draft | 等待 review/release |
| `Upload blocked` | Nacos 上已有 draft 或 reviewing 版本挡住了上传 | 先处理 Nacos 上已有版本 |
| `Conflict` | 本地和 Nacos 都有变化，CLI 不知道该用谁 | 运行 `skill-sync resolve` |

如果你不希望 daemon 自动上传本地改动，启动时关闭：

```bash
nacos-cli skill-sync start --profile team --no-auto-upload
```

关闭后，`Local changes` 不会自动上传。你可以手动上传：

```bash
nacos-cli skill-upload ~/.nacos-cli/skill-repo/pdf --profile team
```

只要这个 skill 已经被 `skill-sync` 跟踪，手动上传和自动上传会走同一套状态逻辑：上传成功后记录 draft version 和 md5，状态变为 `Uploaded`。后续这个版本被发布后，daemon 会校验发布版本的 md5，如果和你上传的内容一致，就把本地状态更新为 `Synced`。

## Upload blocked 怎么处理

如果看到：

```text
pdf    Upload blocked    -    codex,claude,qoder    Nacos draft 0.0.2 exists; review/clear it, auto-upload will retry
```

意思是 Nacos 上已经有一个 draft 或 reviewing 版本。CLI 不会覆盖别人可能正在处理的草稿。

推荐处理方式：

1. 到 Nacos 查看这个 skill 当前的 draft/reviewing 版本。
2. 如果这个版本应该继续推进，就 review/release 它。
3. 如果它已经不需要了，就清理这个 draft。
4. 回到本机等待 daemon 下一次轮询，或者重启 `skill-sync start`。

常用检查和推进命令：

```bash
nacos-cli skill-describe pdf --profile team
nacos-cli skill-review pdf --version 0.0.2 --profile team
nacos-cli skill-release pdf --version 0.0.2 --profile team
```

只要本地仍是 `Upload blocked`，daemon 后续还会重试。状态里的 `NEXT` 会提示当前挡住上传的是 draft 还是 reviewing version。

## Agent 或脚本如何非交互调用

给 Agent 使用时，核心原则是：明确指定 Nacos profile，明确指定冲突选择，不让命令停下来等输入。

从 Nacos 添加并使用默认远端版本：

```bash
nacos-cli skill-sync add pdf --profile team --non-interactive
```

启动 daemon，遇到冲突时记录并跳过：

```bash
nacos-cli skill-sync start --profile team --non-interactive
```

从本地某个 Agent 版本导入：

```bash
nacos-cli skill-sync add pdf --profile team --from codex --non-interactive
```

选择本地最近修改的版本：

```bash
nacos-cli skill-sync add pdf --profile team --from latest --non-interactive
```

用 Nacos 版本解决冲突：

```bash
nacos-cli skill-sync resolve pdf --use-nacos --non-interactive
```

用某个 Agent 的版本解决冲突：

```bash
nacos-cli skill-sync resolve pdf --use-agent codex --non-interactive
```

批量用同一种策略解决所有冲突：

```bash
nacos-cli skill-sync resolve --all --use-nacos --non-interactive
```

非交互模式下，如果只有多个不同的本地版本、但没有告诉 CLI 选哪个，命令会失败并返回非零退出码。这样 Agent 可以把失败暴露出来，而不是悄悄选错。

## status 怎么读

`skill-sync status` 是日常判断入口：

```text
Mode: nacos
Profile: team
Repository: /Users/test/.nacos-cli/skill-repo
Tracking label: latest
Auto-upload: enabled
Sync daemon: running (pid: 12345)

SKILL  STATUS          VERSION  AGENTS                      NEXT
-----  ------          -------  ------                      ----
pdf    Local changes   -        codex,claude,qoder,default  auto-upload pending
```

重点看三列：

| 列 | 怎么看 |
| --- | --- |
| `STATUS` | 当前同步状态，优先处理 `Conflict` 和 `Upload blocked` |
| `AGENTS` | 哪些 Agent 已纳入同步；带 `≠` 的 Agent 表示它和中心副本不同 |
| `NEXT` | 下一步建议，例如等待自动上传、处理 Nacos draft、运行 resolve |

## 常用日常流程

团队成员安装一个 skill：

```bash
nacos-cli skill-sync add pdf --profile team
nacos-cli skill-sync start --profile team
```

本地改完 skill，让 daemon 上传：

```bash
# 编辑任意已链接的 pdf skill
nacos-cli skill-sync status
# 等待 Local changes -> Uploaded
```

维护者发布已上传的版本后，本地等待同步：

```bash
nacos-cli skill-sync status
# Uploaded -> Synced
```

只想本地统一多个 Agent，不接 Nacos：

```bash
nacos-cli skill-sync mode local
nacos-cli skill-sync add pdf
nacos-cli skill-sync start
```

临时跟踪其他 label：

```bash
nacos-cli skill-sync set-label stable
nacos-cli skill-sync start --profile team --refresh
```

只对本次启动覆盖 label：

```bash
nacos-cli skill-sync start --profile team --label stable
```

## 安全边界

`skill-sync` 默认尽量不丢数据：

- 覆盖 Agent 目录前会把原目录备份到 `.skill-sync-backup/`。
- 冲突时默认让你选择，或者记录并跳过。
- `Local changes`、`Uploaded`、`Upload blocked` 这类本地保护状态不会在普通 `start` 中被 Nacos 静默覆盖。
- 非交互模式遇到含糊的本地多版本会失败，而不是随机选择。

如果你明确要用 Nacos 覆盖冲突内容，可以使用：

```bash
nacos-cli skill-sync start --profile team --use-remote-on-conflict
```

如果你只是想强制重新拉取订阅的 skill：

```bash
nacos-cli skill-sync start --profile team --refresh
```

## 排障建议

### 我选了本地版本，为什么还没上传？

先看：

```bash
nacos-cli skill-sync status
```

如果是 `Local changes` 且 `NEXT` 是 `auto-upload pending`，说明 daemon 还没完成上传，或者还在 debounce。确认 daemon 正在运行。

如果是 `Upload blocked`，先处理 Nacos 上已有 draft/reviewing 版本。

### 我选了本地版本，后来好像又变成 Nacos 版本？

确认当前运行的是新版本 CLI，并重启旧 daemon：

```bash
nacos-cli skill-sync stop
nacos-cli skill-sync start --profile team
```

旧 daemon 可能还在用旧逻辑轮询。重启后再用 `status` 看 `Local changes`、`Uploaded` 或 `Synced` 是否符合预期。

### 某个 Agent 没出现在 AGENTS 列里

确认目录存在。如果它不是默认发现路径，手动注册：

```bash
nacos-cli skill-sync agent add my-agent ~/.my-agent/skills
nacos-cli skill-sync agent list
```

### 想从零重新测试

先停止 daemon，再确认你要清理的是测试环境：

```bash
nacos-cli skill-sync stop
```

然后备份或清理 `~/.nacos-cli/skill-sync-state.json`、`~/.nacos-cli/skill-repo`，以及各 Agent skill 目录下测试产生的 skill 或 `.skill-sync-backup/`。生产环境不要直接删除这些路径，先确认有没有真实使用中的 skill。
