# Step 05: 接入 LLM 自动增强笔记

这一阶段新增：

```powershell
go run . add --ai --title "Go map issue" --body "map concurrent read write panic"
```

普通 `add` 仍然完全离线：

```powershell
go run . add --title "标题" --body "正文"
```

只有传 `--ai` 时，程序才会读取配置并调用 LLM。

## 1. 这一阶段做了什么

新增了一个包：

```text
internal/llm
```

它负责：

```text
构造 HTTP 请求
调用 OpenAI-compatible /chat/completions 接口
要求模型返回 JSON
解析 title/body/summary/tags
```

然后 `add` 命令新增：

```text
--ai
```

当你传 `--ai` 时，保存笔记前会先让模型做一次增强。

## 2. 为什么 AI 要做成可选开关

AI 调用有几个特点：

```text
需要 API Key
需要网络
可能产生费用
可能失败
```

所以默认 `add` 不调用 AI。

这样你平时可以稳定地本地记录：

```powershell
go run . add --title "本地笔记" --body "不需要联网"
```

需要 AI 帮忙整理时再加：

```powershell
go run . add --ai --title "草稿" --body "比较乱的内容"
```

## 3. 使用前先配置

先写入配置：

```powershell
go run . config set --api-key "your-api-key" --model "your-chat-model"
```

如果你使用的服务不是默认地址，可以设置：

```powershell
go run . config set --base-url "https://your-provider.example/v1"
```

查看配置：

```powershell
go run . config show
```

默认不会明文显示 API Key。

## 4. add --ai 的执行流程

当你输入：

```powershell
go run . add --ai --title "Go map issue" --body "map concurrent read write panic"
```

程序会这样运行：

```text
1. Cobra 找到 add 命令。
2. add.go 读取 --title、--body、--tag、--ai。
3. 如果没有 --ai，直接保存到 SQLite。
4. 如果有 --ai，读取 config.json。
5. 用 config 里的 api_key/base_url/model 创建 LLM client。
6. 调用 /chat/completions。
7. 要求模型返回 JSON。
8. 解析 JSON 得到 title/body/summary/tags。
9. 用户标签和 AI 标签合并去重。
10. 保存增强后的笔记到 SQLite。
```

## 5. LLM 返回什么

我们要求模型返回 JSON：

```json
{
  "title": "Go map concurrency issue",
  "body": "Polished Markdown body",
  "summary": "说明 Go map 并发读写的处理方式。",
  "tags": ["go", "concurrency", "sync-map"]
}
```

这些字段会保存到数据库：

```text
title      -> notes.title
body       -> notes.body
summary    -> notes.summary
tags       -> notes.tags_json
```

## 6. 为什么不用 SDK

这一阶段没有引入 LLM SDK，而是直接用 Go 标准库：

```text
net/http
encoding/json
```

这样你可以清楚看到：

```text
HTTP 请求怎么构造
Header 怎么设置
JSON body 怎么发送
响应 JSON 怎么解析
```

等你理解这条链路后，再换 SDK 也不迟。

## 7. OpenAI-compatible 是什么意思

项目现在调用的是：

```text
POST {base_url}/chat/completions
```

请求大概是：

```json
{
  "model": "your-chat-model",
  "messages": [
    {"role": "system", "content": "..."},
    {"role": "user", "content": "..."}
  ],
  "temperature": 0.2,
  "response_format": {"type": "json_object"}
}
```

很多服务商都兼容这种接口形态。

所以你不需要把代码写死给某一个服务，只要配置不同的：

```text
base_url
api_key
model
```

## 8. 错误如何处理

如果没有配置 API Key：

```text
ai enhance note: llm api key is empty, run config set --api-key
```

如果没有配置模型：

```text
ai enhance note: llm model is empty, run config set --model
```

如果服务返回非 2xx 状态码，会显示状态码和返回内容。

## 9. 这一阶段你要掌握的重点

```text
1. AI 能力是 add 的可选增强，不影响本地基础功能。
2. config 包提供 API Key、Base URL、Model。
3. llm 包负责 HTTP 请求和 JSON 解析。
4. cli/add.go 负责把增强结果保存到 store。
5. 模型输出不可信，所以要解析和校验。
6. 测试里用 httptest 模拟 LLM 服务，不需要真实联网。
```

## 10. 下一步

下一阶段可以做：

```text
Step 06: 保存 embedding 向量的表结构
Step 07: 调用 embedding model 生成向量
Step 08: 接入 sqlite-vss 做语义检索
```

现在我们已经完成了第一段 AI 链路：

```text
笔记正文 -> LLM -> 摘要/标签/优化正文 -> SQLite
```
