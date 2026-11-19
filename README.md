<div align="center">

<img src="frontend/public/icon.png" width="112" height="112" alt="NexaMap Editor icon" />

# NexaMap Editor v2

**Modern object and map editor for Open Tibia clients — full 32-bit (`uint32`) support from classic `Tibia.dat` / `Tibia.spr` to Tibia 12+ assets.**

[![Repository](https://img.shields.io/badge/REPO-NexaMap--Editorv2-7c4df2?style=for-the-badge&logo=github)](https://github.com/Mateuzkl/NexaMap-Editorv2)
[![CI Windows & Linux](https://img.shields.io/github/actions/workflow/status/Mateuzkl/NexaMap-Editorv2/ci.yml?style=for-the-badge&logo=githubactions&logoColor=white&label=CI%20WINDOWS%20%26%20LINUX)](https://github.com/Mateuzkl/NexaMap-Editorv2/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/Mateuzkl/NexaMap-Editorv2?include_prereleases&style=for-the-badge&logo=github&label=VERSION&color=7c4df2)](https://github.com/Mateuzkl/NexaMap-Editorv2/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Mateuzkl/NexaMap-Editorv2?style=for-the-badge&logo=go&logoColor=white&label=GO&color=00ADD8)](go.mod)

![Windows](https://img.shields.io/badge/WINDOWS-amd64%20%28EXE%20%2B%20NSIS%29-0078D6?style=for-the-badge&logo=windows&logoColor=white)
![Linux](https://img.shields.io/badge/LINUX-amd64%20%28AppImage%20%2B%20DEB%20%2B%20RPM%29-FCC624?style=for-the-badge&logo=linux&logoColor=black)
![macOS](https://img.shields.io/badge/MACOS-arm64%20%7C%20amd64-475569?style=for-the-badge&logo=apple&logoColor=white)
[![License](https://img.shields.io/badge/LICENSE-Apache%202.0%20%2B%20Commons%20Clause-334155?style=for-the-badge)](LICENSE)

<br />

Based on **[OTS Creator](https://github.com/nekiro/ots-creator)** by **Marcin Jałocha (nekiro)** · Customized and maintained by **[Mateuzkl](https://github.com/Mateuzkl)**.

[Downloads & Builds](#downloads--builds) ·
[Features](#features) ·
[Themes & Workspace](#themes--modern-workspace) ·
[Custom Server Converter](#custom-server-converter) ·
[Development](#development) ·
[Credits & License](#credits--license)

</div>

---

## Overview

**NexaMap Editor v2** combines the fast, multi-core **Go + Wails v3 + Svelte 5** engine of **OTS Creator** with a modern workspace experience inspired by NexaMap:

- Select your **Client folder** and **Server folder** once — `items.otb`, `items.xml`, `.otbm`/`.otmap` maps, monsters, and NPCs (TFS XML + Lua revscripts / DBO tables) are detected automatically in read-only mode.
- Full **32-bit (`uint32`, up to `4,294,967,295`)** item, client, and sprite ID support across both the **Object Editor** and **Map Editor**.
- **Parallel multi-core OTBM loader** that opens massive 250+ MB maps in seconds, with configurable performance profiles for low-end or high-end PCs.
- Switch live between **Modern Dark** *(default)*, **Modern Light**, and **Classic Tibia** without restarting.

---

## Downloads & Builds

| Platform | Packages | Where to Get |
|---|---|---|
| **Windows (`amd64`)** | `NexaMap Editor.exe` (portable) · `nexamap-editor-amd64-installer.exe` (NSIS) | [Releases](https://github.com/Mateuzkl/NexaMap-Editorv2/releases) · [CI Artifacts](https://github.com/Mateuzkl/NexaMap-Editorv2/actions/workflows/ci.yml) |
| **Linux (`amd64` / `arm64`)** | `nexamap-editor` · `.AppImage` · `.deb` · `.rpm` | [Releases](https://github.com/Mateuzkl/NexaMap-Editorv2/releases) · [CI Artifacts](https://github.com/Mateuzkl/NexaMap-Editorv2/actions/workflows/ci.yml) |
| **macOS (`arm64` / `amd64`)** | `.app` bundle (`.zip`) | [Releases](https://github.com/Mateuzkl/NexaMap-Editorv2/releases) |

> Every Pull Request into `main` automatically runs **CI Build (Windows & Linux)** (`go vet`, `go test`, `svelte-check`, `vitest`, and native binary/installer packaging).

---

## Features

| Area | Highlights |
|---|---|
| **32-bit (`uint32`) Engine** | Auto-detects & compiles 32-bit `extended-items` `.dat`/`.spr`, 4-byte `items.otb` (`serverId`, `clientId`, `rotateTo`), 32-bit `items.xml`, and hybrid dense + sparse lookup up to `4,294,967,295`. |
| **Clients** | Every `Tibia.dat` / `Tibia.spr` version, `.otfi` features, transparency, improved animations, frame groups, and Tibia 12+ `assets/` (`appearances.dat` + LZMA sheets). |
| **Object Editor** | Items, outfits, effects, missiles, animated live preview, bulk edit, sprite drawing, slicer, optimizer, pixel-based Compare & Merge, `.otobj` / `.obd` import & export. |
| **Map Editor** | Multi-core OTBM & git-friendly OTMAP support, **Find by Server ID / Client ID / Sprite ID**, ServerID / ClientID per-map safety, auto-border brushes, RME brush import, houses, spawns, towns, zones, waypoints, play mode, and live **Multiplayer**. |
| **Performance Profiles** | Configurable **Performance Mode** (`Balanced`, `Low-End PC`, `High Performance`), Undo Queue Size, Undo Max Memory, Worker Threads, and Replace Count in **Settings → General**. |
| **Workspace** | Unified Client + Server cards, read-only resource validation, saved workspace profiles, one-click map launcher, and explicit **Save / Don't Save / Cancel** confirmation dialogs. |
| **Converters** | Built-in **Custom Server Converter** (ServerID → ClientID for `.otbm`/`.otmap`, `items.xml`, `items.otb`), Compile As version converter, and Object Viewer. |
| **Diagnostics** | Live **Debug Console** (`F12`), optional native Windows terminal (`--console`), real-time Go Heap / Sys / Goroutines / GC telemetry, **Force GC**, deterministic WebGL/store lifecycle disposal, and optional `nexamap.log` file output. |
| **AI Agents (MCP)** | Opt-in local MCP server and headless `mcp` stdio mode so AI coding agents can inspect and edit maps alongside you. |

---

## Themes & Modern Workspace

Switch anytime from the top bar or **Settings → Appearance**:

- **Modern Dark** *(default)* — dark glass workspace with custom backdrop, collapsible sidebar, Client/Server cards, resource status chips, and full modern styling across editors and dialogs.
- **Modern Light** — dedicated high-contrast light theme (never distorts sprites or WebGL canvas).
- **Classic Tibia** — preserves the original OTS Creator textures, bevel frames, and pixel fonts 100% intact.
- **Text & UI Zoom (`80%`–`160%`)** — adjust font and interface scale live from the top bar (`−` / `100%` / `+`) or **Settings → Appearance**.

---

## Custom Server Converter

Available in **Workspace → Converters**:

1. Pick a **Source Server Folder** (containing `items.otb`, `items.xml`, and `.otbm`/`.otmap` maps) and a separate **Output Folder**.
2. Click **Analyze Server** to validate `items.otb` mappings and detect any duplicate `ClientID` collisions.
3. Click **Convert to ClientID** to generate converted maps, `items.xml`, and `items.otb` in the output folder — **source files are never overwritten**.

---

## Development

**Requirements:** Go 1.26+, Node 22+, and [`wails3`](https://v3.wails.io) CLI (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`).
On Linux, also install a C compiler and GTK 4 / WebKitGTK 6.0 headers (`build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev`).

```sh
wails3 task dev        # run with hot reload
wails3 task build      # production build into bin/
wails3 task test       # Go + frontend unit tests
```

Launch with a native Windows debug terminal attached:

```sh
.\bin\"NexaMap Editor.exe" --console
```

---

## Credits & License

- **Original Project & Core Engine:** [OTS Creator](https://github.com/nekiro/ots-creator) — **Copyright © 2026 Marcin Jałocha (nekiro)**.
- **License:** Apache License 2.0 with the Commons Clause (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Free to use, modify, and share; not for sale. Derived works must keep all original copyright notices and credits.
- Classic UI graphics and pixel fonts belong to their respective owners (see [NOTICE](NOTICE)).
