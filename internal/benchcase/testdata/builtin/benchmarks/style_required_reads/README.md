# style_required_reads

This built-in case checks whether the model reads multiple required context files before writing.

It measures two behaviors separately:

- there is a style guide in `docs/style.md`
- there is an output contract in `docs/output_contract.md`
- there is a helper in `utils/utils.go`
- the prompt points at the wrong vendor filename, so the model has to locate `FetchDocument`

The required path is:

- read `docs/style.md`
- read `docs/output_contract.md`
- read `utils/utils.go`
- read `vendor/applesmithcorp/client.go`

The recovery path is scored separately:

- `search_text("FetchDocument")` before the first write gets a bonus
- reading missing files after listing the correct directory gets penalized
