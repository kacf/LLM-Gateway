# ⚡ LLM Gateway

> **One command to rule all models.**
>
> Zero-config local LLM server with an OpenAI-compatible API.

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

---

## What is this?

**LLM Gateway** (`llmgw`) is a single-binary tool that lets you run **any GGUF model** from HuggingFace locally with **one command** — no Docker, no Python, no YAML configs, no fuss.

```bash
llmgw run tinyllama
```

That's it. It downloads the model, sets up the inference engine, and gives you a fully OpenAI-compatible REST API at `http://localhost:8080/v1`.

## Features

- 🚀 **One-command setup** — Just `llmgw run <model>` and you're live
- 🤗 **HuggingFace integration** — Auto-downloads GGUF models from the Hub
- 🔌 **OpenAI-compatible API** — Drop-in replacement for `api.openai.com`
- 📦 **Zero dependencies** — Single binary, no Python/Docker/CUDA install needed
- 🧠 **Smart quantization** — Auto-selects the optimal GGUF variant (Q4_K_M)
- 🏷️ **Built-in aliases** — `tinyllama`, `mistral`, `codellama`, `phi2`, and more
- 💾 **Model caching** — Downloads once, serves forever
- 🌐 **CORS enabled** — Use from any web app out of the box
- 🔄 **Streaming support** — Real-time token-by-token responses

## Quick Start

### Install

```bash
# Build from source
git clone https://github.com/kacf/llm-gateway.git
cd llm-gateway
go build -o llmgw .

# Or on Windows
go build -o llmgw.exe .
```

### Run a Model

```bash
# Using a built-in alias
llmgw run tinyllama

# Using a HuggingFace repo ID
llmgw run TheBloke/Mistral-7B-Instruct-v0.2-GGUF

# With custom options
llmgw run mistral -port 9000 -context 8192 -quant Q5_K_M
```

### Use the API

```bash
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "TheBloke/Mistral-7B-Instruct-v0.2-GGUF",
    "messages": [{"role": "user", "content": "Explain quantum computing in one sentence."}]
  }'
```

Works with **any OpenAI-compatible client**:

```python
from openai import OpenAI

client = OpenAI(base_url="http://localhost:8080/v1", api_key="not-needed")

response = client.chat.completions.create(
    model="tinyllama",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)
```

## Commands

| Command | Description |
|---------|-------------|
| `llmgw run <model>` | Download & serve a model |
| `llmgw search <query>` | Search HuggingFace for GGUF models |
| `llmgw list` | List locally cached models |
| `llmgw remove <model>` | Remove a cached model |
| `llmgw aliases` | Show built-in model aliases |
| `llmgw version` | Print version |

## Run Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | API server port |
| `-context` | `4096` | Context window size |
| `-quant` | auto | Preferred quantization (e.g. `Q4_K_M`) |
| `-verbose` | `false` | Show backend logs |
| `-token` | `$HF_TOKEN` | HuggingFace API token |

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/chat/completions` | Chat completion (ChatGPT-style) |
| POST | `/v1/completions` | Text completion |
| GET | `/v1/models` | List available models |
| GET | `/health` | Health check |
| GET | `/` | Server info |

## Built-in Aliases

| Alias | HuggingFace Repo |
|-------|------------------|
| `tinyllama` | TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF |
| `mistral` | TheBloke/Mistral-7B-Instruct-v0.2-GGUF |
| `llama2` | TheBloke/Llama-2-7B-Chat-GGUF |
| `codellama` | TheBloke/CodeLlama-7B-Instruct-GGUF |
| `phi2` | TheBloke/phi-2-GGUF |
| `zephyr` | TheBloke/zephyr-7B-beta-GGUF |
| `deepseek` | TheBloke/deepseek-coder-6.7B-instruct-GGUF |
| ...and more | Run `llmgw aliases` for full list |

## How It Works

```
llmgw run tinyllama
     │
     ├─ 1. Resolve alias → TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF
     ├─ 2. Check local cache (~/.llmgw/models/)
     ├─ 3. Download GGUF from HuggingFace (if needed)
     ├─ 4. Download llama.cpp server (if needed)
     ├─ 5. Start inference engine on internal port
     ├─ 6. Expose OpenAI-compatible API on :8080
     └─ 7. Ready! 🚀
```

## Requirements

- **Go 1.21+** to build
- **4GB+ RAM** for small models (TinyLlama, Phi-2)
- **16GB+ RAM** for 7B models (Mistral, Llama 2)
- Internet connection for first download

## License

MIT
