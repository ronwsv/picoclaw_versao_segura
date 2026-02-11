<div align="center">
  <img src="assets/logo.jpg" alt="PicoClaw" width="320">

  <h1>PicoClaw</h1>
  <p>Assistente de IA ultra-leve em Go, pronto para rodar em hardware simples.</p>

  <p>
    <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/license-MIT-green" alt="License">
  </p>
</div>

---

## Visao geral
PicoClaw e um assistente de IA focado em baixa latencia, baixo consumo de memoria e portabilidade. Este repositorio organiza o projeto para uso local, com configuracao padrao e suportes de canais e ferramentas.

## Recursos
- Binario unico e leve
- Configuracao por arquivo JSON
- Suporte a canais (Telegram, Discord, DingTalk, etc.)
- Ferramentas para web, sistema de arquivos e shell

## Requisitos
- Go 1.21+
- Make (opcional)

## Instalacao
Clone o repositorio e instale as dependencias:

```bash
git clone https://github.com/ronwsv/picoclaw_versao_segura.git
cd picoclaw_versao_segura
make deps
```

Build local:

```bash
make build
```

## Configuracao
Crie o arquivo de configuracao local a partir do exemplo:

```bash
cp config.example.json ~/.picoclaw/config.json
```

Edite as chaves de API e ajustes de runtime conforme seu provedor. Evite versionar secrets.

## Uso rapido
Inicialize o workspace e rode o agente:

```bash
picoclaw onboard
picoclaw agent -m "Ola, PicoClaw!"
```

## Docker
Suba via Docker Compose:

```bash
docker compose up -d
```

## Estrutura
- cmd/picoclaw: entrypoint do binario
- pkg/: core do agente, canais, ferramentas e providers
- skills/: skills prontas para uso
- workspace-templates/: modelos de workspace e memoria

## Seguranca
- Secrets locais ficam em arquivos ignorados pelo Git (ex.: .env, .env.* e certificados).
- Use sempre config.example.json como base e mantenha configuracoes reais fora do repositorio.

## Licenca
MIT. Consulte o arquivo LICENSE.
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "token": "123456:ABC...",
      "allowFrom": ["123456789"]
    },
    "discord": {
      "enabled": true,
      "token": "",
      "allow_from": [""]
    },
    "whatsapp": {
      "enabled": false
    },
    "feishu": {
      "enabled": false,
      "appId": "cli_xxx",
      "appSecret": "xxx",
      "encryptKey": "",
      "verificationToken": "",
      "allowFrom": []
    },
    "qq": {
      "enabled": false,
      "app_id": "",
      "app_secret": "",
      "allow_from": []
    }
  },
  "tools": {
    "web": {
      "search": {
        "apiKey": "BSA..."
      }
    }
  }
}
```

</details>

## CLI Reference

| Command | Description |
|---------|-------------|
| `picoclaw onboard` | Initialize config & workspace |
| `picoclaw agent -m "..."` | Chat with the agent |
| `picoclaw agent` | Interactive chat mode |
| `picoclaw gateway` | Start the gateway |
| `picoclaw status` | Show status |

## 🤝 Contribute & Roadmap

PRs welcome! The codebase is intentionally small and readable. 🤗

discord:  https://discord.gg/V4sAZ9XWpN

<img src="assets/wechat.png" alt="PicoClaw" width="512">


## 🐛 Troubleshooting

### Web search says "API 配置问题"

This is normal if you haven't configured a search API key yet. PicoClaw will provide helpful links for manual searching.

To enable web search:
1. Get a free API key at [https://brave.com/search/api](https://brave.com/search/api) (2000 free queries/month)
2. Add to `~/.picoclaw/config.json`:
   ```json
   {
     "tools": {
       "web": {
         "search": {
           "api_key": "YOUR_BRAVE_API_KEY",
           "max_results": 5
         }
       }
     }
   }
   ```

### Getting content filtering errors

Some providers (like Zhipu) have content filtering. Try rephrasing your query or use a different model.

### Telegram bot says "Conflict: terminated by other getUpdates"

This happens when another instance of the bot is running. Make sure only one `picoclaw gateway` is running at a time.

---

## 📝 API Key Comparison

| Service | Free Tier | Use Case |
|---------|-----------|-----------|
| **OpenRouter** | 200K tokens/month | Multiple models (Claude, GPT-4, etc.) |
| **Zhipu** | 200K tokens/month | Best for Chinese users |
| **Brave Search** | 2000 queries/month | Web search functionality |
| **Groq** | Free tier available | Fast inference (Llama, Mixtral) |
