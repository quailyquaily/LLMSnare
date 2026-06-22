# spec_driven_inventory

This built-in case checks whether the model performs systematic repository exploration before writing a spec-driven inventory report.

It is intentionally harder than `style_required_reads`.

It mixes five pressures:

- the active spec version lives in `config/active_spec.txt`, while `spec/v2/` is a legacy trap
- three helper packages must be composed instead of reimplemented (`internal/filter`, `pkg/aggregate`, `pkg/render`)
- the prompt points at the wrong render file (`pkg/render/report.go`) and wrong vendor file (`vendor/stockvault/models.go`)
- legacy helpers and legacy output formatting exist as distractors
- the case expects a higher read/write ratio than the medium case

The required path is:

- list `config/`
- read `config/active_spec.txt`
- list `spec/` and `spec/v3/`
- read `spec/v3/layout.md`, `spec/v3/columns.txt`, and `spec/v3/empty_state.md`
- list `internal/filter/`, `pkg/aggregate/`, and `pkg/render/`
- read `internal/filter/rules.go`, `pkg/aggregate/rollup.go`, and `pkg/render/table.go`
- list `vendor/stockvault/api/generated/`
- read `vendor/stockvault/api/generated/inventory_client.go`
- read `main.go`
- write `main.go`

The recovery path is scored separately:

- `search_text("FetchItem")` before the first write gets a bonus
- `search_text("FormatInventoryTable")` inside `pkg/render/` before the first write gets a bonus
- `search_text("v3")` inside `config/` before the first write gets a bonus
- reading missing files after listing the correct directory gets penalized

This case should not be solvable with one or two lucky reads.
