# Project objectives
- A context-efficient tool for LLM agents to interact with the web on the user's behalf.

# Requirements
- Written in the latest version of the Go language.
- Install/uninstall/upgrade a private local copy of Chromium.
- Create/delete, start/stop, list any number of independent Chromium processes.
- Exchange raw JSON CDP messages with a Chromium process over WebSockets.

# Technical hints
- Use the Cobra library for the CLI.
- Discover Chromium versions via https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json
- Do not focus on writing tests or documentation at this time.

# Reference material
- [Chromium Source Code](reference/chromium)
- [Chrome DevTools Frontend Source Code](reference/devtools-frontend)
- [Chromium DevTools Protocol](reference/devtools-protocol)

# Codebase style
- High density
- Flat organization
- Minimalistic
- Explicitly wired
- Internally consistent
- Artisanal