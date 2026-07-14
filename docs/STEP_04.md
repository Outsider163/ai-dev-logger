# Step 04: 配置文件和 LLM 参数管理

这一阶段新增配置能力，但还不真正调用 LLM。

新增命令：

```powershell
go run . config path
go run . config show
go run . config set --api-key "your-api-key" --model "your-chat-model"
```

## 1. 为什么需要配置文件

后面接入 LLM 时，程序需要知道这些信息：

```text
API Key
API Base URL
Chat Model
Embedding Model
```

这些内容不适合写死在代码里。

原因有三个：

```text
1. API Key 是秘密，不能提交到代码仓库。
2. 不同用户可能用不同模型。
3. 以后更换服务商时，不应该改业务代码。
```

所以我们新增了配置文件。

## 2. 配置文件默认放在哪里

你可以运行：

```powershell
go run . config path
```

它会打印当前配置文件路径。

默认情况下，配置文件会放在用户配置目录下：

```text
用户配置目录/ai-dev-logger/config.json
```

如果你只是练习，可以用 `--config` 指定一个临时配置文件：

```powershell
go run . --config .\.tmp\config.json config set --model "test-model"
```

## 3. 当前配置结构

配置文件是 JSON：

```json
{
  "llm": {
    "api_key": "your-api-key",
    "base_url": "https://api.openai.com/v1",
    "model": "your-chat-model",
    "embedding_model": "your-embedding-model"
  }
}
```

Go 代码里对应：

```go
type Config struct {
	LLM LLMConfig `json:"llm"`
}

type LLMConfig struct {
	APIKey         string `json:"api_key"`
	BaseURL        string `json:"base_url"`
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}
```

## 4. config set 怎么工作

当你输入：

```powershell
go run . config set --api-key "abc" --model "demo-model"
```

程序会这样执行：

```text
1. Cobra 找到 config 命令。
2. Cobra 继续找到 set 子命令。
3. CLI 检查用户传了哪些 flag。
4. 读取旧配置文件，如果文件不存在就使用默认配置。
5. 只更新用户传入的字段。
6. 把配置写回 JSON 文件。
```

这和 `update note` 很像：

```text
先读旧值
  -> 替换新值
  -> 保存
```

## 5. config show 为什么隐藏 API Key

`config show` 默认会隐藏 API Key：

```text
llm.api_key: sk-1...abcd
```

或者：

```text
llm.api_key: ********
```

这是因为密钥不应该随便显示在终端里，尤其是你截图、录屏、直播或者复制日志时。

如果你确实要明文查看，可以加：

```powershell
go run . config show --reveal
```

实际项目里要谨慎使用 `--reveal`。

## 6. config path 的作用

`config path` 很简单，只打印配置文件路径：

```powershell
go run . config path
```

它对初学者很有用，因为你可以明确知道配置到底写到哪里了。

## 7. internal/config 包负责什么

这一阶段新增：

```text
internal/config/config.go
```

它负责：

```text
默认配置
默认配置路径
读取 JSON
保存 JSON
API Key 脱敏显示
```

CLI 层不直接操作 JSON 文件。

这和前面的 store 层类似：

```text
internal/cli      负责命令行交互
internal/store    负责数据库
internal/config   负责配置文件
```

## 8. 为什么 Save 使用 0600 权限

保存配置时使用：

```go
os.WriteFile(path, data, 0o600)
```

`0o600` 表示：

```text
文件所有者可读写
其他用户没有权限
```

这是保存 API Key 时的基本保护。

注意：这不是加密，只是文件权限控制。

## 9. 这一阶段你要掌握的重点

```text
1. 配置不应该硬编码在代码里。
2. API Key 不应该提交到 Git 仓库。
3. CLI 层负责解析 config set/show/path。
4. config 包负责 JSON 文件读写。
5. show 默认隐藏 API Key。
6. --config 可以指定临时配置文件，方便测试。
```

## 10. 下一步

下一阶段可以开始接 LLM 客户端：

```text
读取 config
  -> 检查 api_key/model 是否存在
  -> 调用 LLM API
  -> 返回 summary 和 tags
  -> 保存到 notes.summary 和 notes.tags_json
```

到这里，AI 能力需要的配置地基已经准备好了。
