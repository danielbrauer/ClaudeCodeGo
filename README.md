# ClaudeCodeGo

A drop-in Go reimplementation of the [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI (`@anthropic-ai/claude-code`).

## What is this?

This project aims to reimplement the official Claude Code CLI in Go with full feature parity. The resulting binary is intended to be a complete replacement — same commands, same tools, same config file locations, same behavior — so users can swap it in without changing their workflows.

The implementation targets **Claude subscription auth** (Pro/Team/Enterprise via OAuth), not API key auth. It communicates with the same backend the official CLI uses.

## Building

Requires Go 1.24+.

```sh
go build ./cmd/claude
```

This produces a single static `claude` binary. Cross-compilation works for macOS (arm64/amd64), Linux (amd64/arm64), and Windows (amd64).

## Status

This is a work in progress. See `CLAUDE.md` for detailed architecture notes, implementation phases, and the full reverse-engineering guide.

## License

MIT
