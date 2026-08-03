# Language server

`fhirlint lsp` runs a [Language Server Protocol](https://microsoft.github.io/language-server-protocol/) server over stdio, so validation findings appear in your editor while you author instead of only when you run the CLI.

It is meant to be launched by an editor, not by hand.

## What you get

- **Diagnostics** on every open FHIR resource, republished as you type. The HL7 message id is the diagnostic code, so it shows in the problems view and can be searched for.
- **Hover** on a finding renders the same offline explanation `fhirlint explain` prints — no network, no JAR round trip.
- **Quick fix** to write a suppression into `fhirlint.yml`.

Everything the CLI applies is applied here too: `fhirlint.yml`, profiles, IGs and suppression rules. What the editor shows is what CI will say.

## Why it is fast

A cold `fhirlint validate` spends tens of seconds starting a JVM and loading packages before it looks at a resource — unusable in an edit loop. `fhirlint lsp` starts a validator server once (the same one behind [`fhirlint serve`](daemon.md)) and keeps it warm for the session, so validating a buffer takes milliseconds.

Share one warm validator across editors by starting it yourself:

```bash
fhirlint serve --port 8080 --ig hl7.fhir.us.core#9.0.0
fhirlint lsp --server http://localhost:8080
```

## Editor setup

### Neovim

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "json", "xml" },
  callback = function()
    vim.lsp.start({
      name = "fhirlint",
      cmd = { "fhirlint", "lsp" },
      root_dir = vim.fs.dirname(vim.fs.find({ "fhirlint.yml", ".git" }, { upward = true })[1]),
    })
  end,
})
```

### VS Code

There is no extension yet. Any generic LSP bridge extension works; point it at `fhirlint lsp` for the `json` and `xml` language ids.

### Helix

In `languages.toml`:

```toml
[language-server.fhirlint]
command = "fhirlint"
args = ["lsp"]

[[language]]
name = "json"
language-servers = ["vscode-json-language-server", "fhirlint"]
```

## Options

| Flag | Meaning |
|------|---------|
| `--server <url>` | Use an already-running validator server instead of starting one |
| `--fhir-version` | FHIR version to validate against |
| `--ig` | IG package to load (repeatable) |
| `--profile` | Profile applied to every validated document (repeatable) |
| `--no-suppress-action` | Do not offer the quick fix that writes to `fhirlint.yml` |

`--fhir-version`, `--ig` and `--profile` fall back to `fhirlint.yml` when not given, so a project that already has a config needs no editor-side configuration.

## Behaviour worth knowing

**Edits are debounced.** Validation runs 400 ms after you stop typing. Without that, a keystroke would queue work faster than it completes.

**A backend failure does not clear your diagnostics.** If the validator server dies or times out, the previous findings stay on screen. Clearing them would look like the file just became clean, which is the one thing a linter must never imply.

**Ranges are approximate.** The validator reports a position, not a span, so a finding highlights from that position to the end of the line. When the reported column falls outside the line — the validator counts positions against the resource as it parsed it, which need not match a reformatted buffer — the whole line is marked instead. That is deliberate: an empty range is invisible and cannot be hovered.

**Only `file://` documents are validated.** Untitled and remote buffers are ignored rather than erroring.

**Suppressions are written as text.** The quick fix appends to an existing `suppress:` block or creates one, editing `fhirlint.yml` as text rather than re-encoding it — a config carries comments explaining why each suppression exists, and a YAML round trip would delete them. Applying the same suppression twice does nothing.

## Diagnostics vs. the CLI

Diagnostics come from the warm validator server, which fixes its FHIR version, IGs and terminology settings at startup. Per-document options that change validation semantics therefore come from how the language server was started, not from the buffer.

Suppressed findings are not shown. A suppression is a decision the project already made; surfacing it in the editor would undo the point of having made it.

## Troubleshooting

The server logs to stderr, which most editors expose as the LSP server log. Startup looks like:

```
fhirlint lsp: starting validator server…
fhirlint lsp: ready (validator at http://localhost:42593)
```

If the first validation takes a minute, the validator JAR is being downloaded (~250 MB, once). Run `fhirlint validate` on any file first to prime it.
