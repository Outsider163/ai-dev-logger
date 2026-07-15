# Step 14：构建与 V1 交付

V1 已完成。现在你不需要每次都用 `go run .`，可以构建 Windows 可执行文件：

```powershell
$env:GOTOOLCHAIN='local'
$env:GOCACHE='D:\SoftWare\Intelligent_Agent_Project\ai-dev-logger\.gocache'
go build -o dist\ai-dev-logger.exe .
```

构建成功后，文件位于：

```text
dist\ai-dev-logger.exe
```

运行：

```powershell
.\dist\ai-dev-logger.exe --help
.\dist\ai-dev-logger.exe list
```

`dist/` 已在 `.gitignore` 中。它是构建产物，不应该提交到源码仓库。

## 1. 首次使用最短路径

```powershell
# 1. 配置模型（只在需要 AI 或语义检索时执行）
.\dist\ai-dev-logger.exe config set --api-key "your-api-key" --model "your-chat-model" --embedding-model "your-embedding-model"

# 2. 新增笔记，同时生成向量
.\dist\ai-dev-logger.exe add --embed --title "Go mutex" --tag go --body "Use sync.Mutex to protect shared state."

# 3. 查看索引是否完整
.\dist\ai-dev-logger.exe status

# 4. 语义检索
.\dist\ai-dev-logger.exe semantic "how to protect shared data" --explain
```

只想记录普通笔记、不使用模型时：

```powershell
.\dist\ai-dev-logger.exe add --title "SQLite transaction" --tag sqlite --body "Use a transaction for related writes."
.\dist\ai-dev-logger.exe list
.\dist\ai-dev-logger.exe search transaction
```

## 2. 数据在哪里

默认情况下，Windows 会把数据放在：

```text
C:\Users\<你的用户名>\AppData\Roaming\ai-dev-logger\notes.db
C:\Users\<你的用户名>\AppData\Roaming\ai-dev-logger\config.json
```

可以确认实际路径：

```powershell
.\dist\ai-dev-logger.exe config path
```

数据库文件 `notes.db` 包含你的笔记和向量。配置文件包含 API Key，因此不要分享或提交它。

## 3. 备份与迁移

本项目没有云端服务，备份就是复制 SQLite 数据库文件：

```powershell
Copy-Item "$env:APPDATA\ai-dev-logger\notes.db" "$env:USERPROFILE\Documents\ai-dev-logger-backup.db"
```

迁移到新电脑时：

```text
1. 安装或复制 ai-dev-logger.exe。
2. 把 notes.db 复制到新电脑的 AppData\Roaming\ai-dev-logger\ 目录。
3. 重新设置 config.json 中的 API Key，或运行 config set。
4. 如果换了 embedding 模型，运行 embed --all。
```

## 4. V1 功能清单

```text
add / list / show / update / delete
search：SQLite LIKE 关键词检索
config：保存 API 与模型配置
add --ai：AI 润色、摘要、标签
embed <id> / embed --all：生成或重建向量
status：检查当前模型的索引完整性
semantic <query>：本地向量语义检索
semantic <query> --explain：对命中笔记生成 AI 解读
add --embed：新增笔记后立即建立向量
```

## 5. 发布前检查

每次交付前执行：

```powershell
$env:GOTOOLCHAIN='local'
$env:GOCACHE='D:\SoftWare\Intelligent_Agent_Project\ai-dev-logger\.gocache'
go test ./...
go vet ./...
go build -o dist\ai-dev-logger.exe .
.\dist\ai-dev-logger.exe --help
```

这四项分别检查：测试、静态问题、构建、启动。

## 6. 学习路线回顾

```text
Step 01-05  Cobra、SQLite、CRUD、配置、AI 笔记整理
Step 06-07  向量表、Embeddings API
Step 08-09  本地语义检索、AI 解读
Step 10-12  向量生命周期、状态检查、自动建索引
Step 13     端到端验收
Step 14     构建、使用、备份和交付
```

到这里，你已经完成了一个能本地使用的 AI 开发日志助手 V1。
