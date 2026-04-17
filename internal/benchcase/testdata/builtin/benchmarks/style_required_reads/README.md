# style_required_reads

This built-in case checks whether the model reads multiple required context files before writing.

It is intentionally harder than the smoke sample.

It mixes four pressures:

- there are both current and legacy docs in `docs/`, and the current docs live one level deeper
- there is a helper in `utils/utils.go`
- there is also a current summary helper and a legacy summary helper
- the prompt points at the wrong vendor filename, and the real `FetchDocument` lives deeper under `vendor/applesmithcorp/generated/`

The required path is:

- list `docs/`
- list `docs/current/`
- read `docs/current/style.md`
- read `docs/current/output_contract.md`
- list `utils/`
- read `utils/utils.go`
- read `utils/summary_current.go`
- list `vendor/applesmithcorp/`
- list `vendor/applesmithcorp/generated/`
- read `vendor/applesmithcorp/generated/client_generated.go`

The recovery path is scored separately:

- `search_text("FetchDocument")` before the first write gets a bonus
- `search_text("(no documents)")` inside `utils/` before the first write also gets a bonus
- reading missing files after listing the correct directory gets penalized

This case is meant to stay explainable, but it should no longer be solvable by one or two lucky reads.
