-- Minimal auto-setup: register the gsx filetype on load.
-- Users still need to call require("gsx").setup() from their config
-- to enable tree-sitter parser registration and the LSP.
vim.filetype.add({
	extension = {
		gsx = "gsx",
	},
})
