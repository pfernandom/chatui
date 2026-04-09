local M = {}

--- Find the root of this plugin (editor/nvim/) on the runtimepath.
local function plugin_root()
	for _, p in ipairs(vim.api.nvim_list_runtime_paths()) do
		if vim.uv.fs_stat(p .. "/lua/gsx/init.lua") then
			return p
		end
	end
	return nil
end

--- Find the tree-sitter-gsx source directory.
--- It sits alongside the nvim plugin at ../tree-sitter-gsx.
local function grammar_src_dir()
	local root = plugin_root()
	if not root then
		return nil
	end
	local src = vim.fs.normalize(root .. "/../tree-sitter-gsx/src")
	if vim.uv.fs_stat(src .. "/parser.c") then
		return src
	end
	return nil
end

--- Build the tree-sitter parser .so and install it where Neovim can find it.
--- Returns nil on success, or an error string on failure.
function M.build_parser()
	local src = grammar_src_dir()
	if not src then
		return "Could not find tree-sitter-gsx/src/parser.c relative to the plugin"
	end

	local parser_dir = vim.fn.stdpath("data") .. "/site/parser"
	vim.fn.mkdir(parser_dir, "p")
	local output = parser_dir .. "/gsx.so"

	-- Use the same flags nvim-treesitter uses on macOS vs Linux
	local cc = vim.fn.exepath("cc")
	if cc == "" then
		cc = vim.fn.exepath("gcc")
	end
	if cc == "" then
		return "No C compiler found (cc or gcc)"
	end

	local cmd = {
		cc,
		"-o", output,
		"-I", src,
		src .. "/parser.c",
		"-O2",
	}

	if vim.fn.has("mac") == 1 then
		table.insert(cmd, "-bundle")
	else
		table.insert(cmd, "-shared")
		table.insert(cmd, "-fPIC")
	end

	local result = vim.system(cmd, { text = true }):wait()
	if result.code ~= 0 then
		return string.format("Compiler failed (exit %d): %s", result.code, result.stderr or "")
	end

	return nil
end

--- Check if the gsx parser .so is installed.
function M.parser_installed()
	local path = vim.fn.stdpath("data") .. "/site/parser/gsx.so"
	return vim.uv.fs_stat(path) ~= nil
end

--- Set up GSX support for Neovim.
---
--- Registers the gsx filetype and tree-sitter language, configures the LSP,
--- and auto-builds the parser if it isn't installed yet.
---
--- Options:
---   lsp.cmd       string[]  Command to start the LSP server (default: { "tui", "lsp" })
---   lsp.enabled   boolean   Whether to start the LSP (default: true)
---   lsp.log       string    Path to LSP log file for debugging (optional)
---
--- Example:
---   require("gsx").setup()
---   require("gsx").setup({ lsp = { cmd = { "/path/to/tui", "lsp" } } })
---   require("gsx").setup({ lsp = { log = "/tmp/gsx-lsp.log" } })
function M.setup(opts)
	opts = opts or {}
	opts.lsp = opts.lsp or {}

	local lsp_enabled = opts.lsp.enabled ~= false

	-- Register gsx as a tree-sitter language so Neovim knows which parser to load.
	if vim.treesitter.language and vim.treesitter.language.register then
		vim.treesitter.language.register("gsx", "gsx")
	end

	-- Auto-build the parser if it isn't installed yet.
	if not M.parser_installed() then
		local err = M.build_parser()
		if err then
			vim.notify("[gsx] Failed to build parser: " .. err, vim.log.levels.WARN)
		end
	end

	-- :GSXBuildParser command for manual rebuilds.
	vim.api.nvim_create_user_command("GSXBuildParser", function()
		local err = M.build_parser()
		if err then
			vim.notify("[gsx] " .. err, vim.log.levels.ERROR)
		else
			vim.notify("[gsx] Parser built successfully", vim.log.levels.INFO)
		end
	end, { desc = "Build and install the GSX tree-sitter parser" })

	-- Configure the LSP server.
	if lsp_enabled then
		local cmd = opts.lsp.cmd or { "tui", "lsp" }
		if opts.lsp.log then
			cmd = vim.deepcopy(cmd)
			table.insert(cmd, "--log")
			table.insert(cmd, opts.lsp.log)
		end

		-- Neovim 0.11+ native LSP config
		if vim.lsp.config then
			vim.lsp.config("gsx", {
				cmd = cmd,
				filetypes = { "gsx" },
				root_markers = { "go.mod", "go.sum" },
			})
			vim.lsp.enable("gsx")
		else
			-- Fallback: set up an autocommand that starts the LSP on BufEnter
			vim.api.nvim_create_autocmd("FileType", {
				pattern = "gsx",
				callback = function(ev)
					vim.lsp.start({
						name = "gsx",
						cmd = cmd,
						root_dir = vim.fs.root(ev.buf, { "go.mod", "go.sum" }),
					})
				end,
			})
		end
	end
end

return M
