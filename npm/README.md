# jevx

An agent skill for Claude Code, Codex and any agent, backed by a small CLI: fast typed judgements (yes/no, pick-one,
rating) from any Jev-style System One endpoint.

```bash
npm i -g @muthuishere/jevx          # downloads the jevx binary from GitHub Releases, installs the skill + hook entries (plugins off)
jevx profile add jev https://your-endpoint/v1/systemone --model MODEL --header "Authorization: Bearer $YOUR_KEY_VAR"
echo "Prod is down" | jevx is "Is this urgent?"
```

`JEVX_NO_HOOK=1 npm i -g @muthuishere/jevx` installs the skills only. Docs: https://muthuishere.github.io/jevx/
