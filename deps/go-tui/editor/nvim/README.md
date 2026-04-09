# GSX Neovim Plugin

Neovim support for `.gsx` files: tree-sitter syntax highlighting, Go language injection, and LSP integration.

## Requirements

- Neovim 0.9+ (0.11+ recommended for native LSP config)
- [nvim-treesitter](https://github.com/nvim-treesitter/nvim-treesitter) for syntax highlighting
- The `tui` CLI installed and on your `$PATH` (for the LSP)
- A C compiler for building the tree-sitter parser (`cc` or `gcc`)

## Install

### lazy.nvim

Since the plugin lives in a subdirectory of the repo, use the `init` callback to prepend the correct path:

```lua
{
  "grindlemire/go-tui",
  init = function()
    -- Add the nvim plugin subdirectory to the runtimepath
    vim.opt.rtp:prepend(vim.fn.stdpath("data") .. "/lazy/go-tui/editor/nvim")
  end,
  config = function()
    require("gsx").setup()
  end,
  ft = "gsx",
}
```

### packer.nvim

```lua
use {
  "grindlemire/go-tui",
  rtp = "editor/nvim",
  config = function()
    require("gsx").setup()
  end,
  ft = { "gsx" },
}
```

### vim-plug

```vim
Plug 'grindlemire/go-tui', { 'rtp': 'editor/nvim' }
```

Then in your `init.lua`:

```lua
require("gsx").setup()
```

### Manual

Clone the repo and add the plugin path to your runtimepath:

```lua
vim.opt.rtp:prepend("/path/to/go-tui/editor/nvim")
require("gsx").setup()
```

## Setup

After installing, call `setup()` in your Neovim config:

```lua
require("gsx").setup()
```

This does three things:

1. Registers `.gsx` as a filetype
2. Registers the tree-sitter parser with nvim-treesitter (so `:TSInstall gsx` works)
3. Configures and starts the `tui lsp` language server for `.gsx` files

## Options

```lua
require("gsx").setup({
  lsp = {
    enabled = true,                       -- set to false to disable the LSP
    cmd = { "tui", "lsp" },              -- command to start the LSP server
    log = "/tmp/gsx-lsp.log",            -- optional: enable LSP debug logging
  },
})
```

## Installing the tree-sitter parser

After setup, install the parser:

```vim
:TSInstall gsx
```

If that doesn't work (the grammar isn't upstream in nvim-treesitter yet), the plugin's parser registration should still let nvim-treesitter build it from the GitHub repo. You can verify with:

```vim
:TSInstallInfo
```

Look for `gsx` in the list.

## LSP features

The `tui lsp` language server provides:

- Real-time diagnostics
- Hover documentation
- Auto-completion (elements, attributes, Tailwind classes, Go expressions via gopls)
- Go-to-definition
- Find references
- Document and workspace symbols
- Semantic token highlighting
- Code formatting

Make sure the `tui` binary is installed and on your `$PATH`:

```bash
go install github.com/grindlemire/go-tui/cmd/tui@latest
```

## Troubleshooting

**No syntax highlighting**: Run `:TSInstall gsx` and restart Neovim. Check `:TSInstallInfo` to confirm the parser is installed.

**LSP not starting**: Check that `tui lsp` runs from your shell. Enable logging with `lsp = { log = "/tmp/gsx-lsp.log" }` and inspect the log. Run `:LspInfo` (or `:lua vim.print(vim.lsp.get_clients())` on 0.11+) to see if the client attached.

**Wrong filetype**: Run `:set ft?` in a `.gsx` buffer. It should say `filetype=gsx`. If not, make sure the plugin loaded (`:scriptnames` should show `gsx`).
